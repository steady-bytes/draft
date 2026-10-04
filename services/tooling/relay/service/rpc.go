package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"
	linemanv1Connect "github.com/steady-bytes/draft/api/tooling/lineman/v1/v1connect"
	relayv1 "github.com/steady-bytes/draft/api/tooling/relay/v1"
	relayv1Connect "github.com/steady-bytes/draft/api/tooling/relay/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// audioChunkBytes is how much of a track's file StreamAudioOut reads and sends per AudioChunk.
const audioChunkBytes = 32 * 1024

// lowConfidenceThreshold marks a finalized recording RECORDING_STATUS_LOW_CONFIDENCE rather than
// RECORDING_STATUS_COMPLETE when its segments' average whisper.cpp confidence (see transcribe.go's
// avg_logprob-derived Confidence) falls below this. A recording with no segments at all (stopped
// immediately, or genuine silence throughout) is COMPLETE, not LOW_CONFIDENCE -- see
// RecordingSession.Stats's own doc.
const lowConfidenceThreshold = 0.5

type (
	Handler interface {
		chassis.RPCRegistrar
		relayv1Connect.RelayServiceHandler
	}
	handler struct {
		// UnimplementedRelayServiceHandler answers CodeUnimplemented for every RPC this file
		// doesn't define below -- everything past Phase 3's own scope (devices, queue, recording,
		// transcripts, archive, action items). See the implementation plan's phase list for when
		// each one is filled in; there is no separate hand-written stub per method to keep in
		// sync as RPCs get implemented phase by phase.
		relayv1Connect.UnimplementedRelayServiceHandler

		logger     chassis.Logger
		model      Model
		player     *Player
		storageDir string

		// Phase 4 (Recording + live transcription) -- see session.go for the concurrency/format
		// rules these share.
		recordings  RecordingModel
		sessions    *SessionManager
		transcriber Transcriber
		events      EventPublisher

		// Phase 6 (Action items -> Lineman).
		lineman linemanv1Connect.LinemanServiceClient

		// Phase 8 (Speaker diarization -- archive only, see diarize.go).
		diarizer Diarizer

		// Phase 9 (Native device support -- listing only this phase, see devices.go).
		devices DeviceLister
	}
)

func NewHandler(
	logger chassis.Logger,
	model Model,
	player *Player,
	storageDir string,
	recordings RecordingModel,
	sessions *SessionManager,
	transcriber Transcriber,
	events EventPublisher,
	lineman linemanv1Connect.LinemanServiceClient,
	diarizer Diarizer,
	devices DeviceLister,
) Handler {
	return &handler{
		logger:      logger,
		model:       model,
		player:      player,
		storageDir:  storageDir,
		recordings:  recordings,
		sessions:    sessions,
		transcriber: transcriber,
		events:      events,
		lineman:     lineman,
		diarizer:    diarizer,
		devices:     devices,
	}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := relayv1Connect.NewRelayServiceHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, handler, true)
}

// -- Library (Phase 3) -----------------------------------------------------------------------------
//
// Each RPC below starts its own child span, following the pattern established in
// services/examples/crud-event/service/rpc.go: NewTraceInterceptor's own request-level span has no
// hook for setting business/runtime attributes, so a StartSpan child is what actually makes them
// (and the log line) reach the WideEvent that flows to Beacon.

func (h *handler) ListTracks(ctx context.Context, req *connect.Request[relayv1.ListTracksRequest]) (*connect.Response[relayv1.ListTracksResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.list_tracks")
	span.SetBusinessAttribute("filter", req.Msg.GetFilter())

	tracks, err := h.model.ListTracks(ctx, req.Msg.GetFilter())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to list tracks")
		span.End(err)
		return nil, err
	}
	span.SetRuntimeAttribute("track_count", strconv.Itoa(len(tracks)))
	span.End(nil)

	return connect.NewResponse(&relayv1.ListTracksResponse{Tracks: tracks}), nil
}

func (h *handler) AddTrack(ctx context.Context, req *connect.Request[relayv1.AddTrackRequest]) (*connect.Response[relayv1.Track], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.add_track")
	span.SetBusinessAttribute("title", req.Msg.GetTitle())

	track, err := h.model.AddTrack(ctx, req.Msg)
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to add track")
		span.End(err)
		return nil, err
	}
	span.SetBusinessAttribute("track_id", track.GetId())
	span.End(nil)

	return connect.NewResponse(track), nil
}

func (h *handler) UploadTrack(ctx context.Context, stream *connect.ClientStream[relayv1.UploadTrackChunk]) (*connect.Response[relayv1.Track], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.upload_track")

	// Bytes are written straight to a temp file under storageDir as chunks arrive rather than
	// buffered in memory -- an upload can be an arbitrarily large recording, and the whole point
	// of streaming the RPC is to not need it all resident at once.
	tmp, err := os.CreateTemp(h.storageDir, "upload-*.wav")
	if err != nil {
		span.End(err)
		return nil, fmt.Errorf("failed to create temp upload file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once SaveUploadedTrack has renamed the file into place

	var meta *relayv1.AddTrackRequest
	var size int64
	for stream.Receive() {
		chunk := stream.Msg()
		if chunk.GetMetadata() != nil {
			meta = chunk.GetMetadata()
		}
		if data := chunk.GetData(); len(data) > 0 {
			n, werr := tmp.Write(data)
			size += int64(n)
			if werr != nil {
				tmp.Close()
				span.End(werr)
				return nil, fmt.Errorf("failed to write uploaded bytes: %w", werr)
			}
		}
	}
	if err := stream.Err(); err != nil {
		tmp.Close()
		span.End(err)
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		span.End(err)
		return nil, err
	}

	span.SetRuntimeAttribute("upload_bytes", strconv.FormatInt(size, 10))
	if meta != nil {
		span.SetBusinessAttribute("title", meta.GetTitle())
	}

	track, err := h.model.SaveUploadedTrack(ctx, tmpPath, meta, "")
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to save uploaded track")
		span.End(err)
		return nil, err
	}
	span.SetBusinessAttribute("track_id", track.GetId())
	span.End(nil)

	return connect.NewResponse(track), nil
}

func (h *handler) DeleteTrack(ctx context.Context, req *connect.Request[relayv1.DeleteTrackRequest]) (*connect.Response[relayv1.DeleteTrackResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.delete_track")
	trackID := req.Msg.GetTrackId()
	span.SetBusinessAttribute("track_id", trackID)

	// Stop it first if it's the one currently loaded on the shared browser output -- otherwise
	// StreamAudioOut's send loop keeps trying to read bytes from a file that's about to disappear
	// out from under it mid-stream.
	if h.player.State().GetTrackId() == trackID {
		h.player.Pause()
	}

	if err := h.model.DeleteTrack(ctx, trackID); err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to delete track")
		span.End(err)
		return nil, err
	}
	span.End(nil)
	return connect.NewResponse(&relayv1.DeleteTrackResponse{}), nil
}

// -- Playback (Phase 3) -----------------------------------------------------------------------------
//
// Play/Pause/Seek/SetVolume/GetPlayerState/StreamAudioOut all operate on the single shared
// browser-output Player (player.go) regardless of what output_device_id a request names -- see
// BrowserOutputDeviceID's own comment.

func (h *handler) Play(ctx context.Context, req *connect.Request[relayv1.PlayRequest]) (*connect.Response[relayv1.PlayerState], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.play")
	span.SetBusinessAttribute("track_id", req.Msg.GetTrackId())
	span.SetBusinessAttribute("output_device_id", req.Msg.GetOutputDeviceId())

	if trackID := req.Msg.GetTrackId(); trackID != "" {
		if _, err := h.model.GetTrack(ctx, trackID); err != nil {
			h.logger.WithContext(ctx).WithError(err).WithField("track_id", trackID).Error("play: unknown track")
			span.End(err)
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("track %q not found: %w", trackID, err))
		}
	}

	state := h.player.Play(req.Msg.GetTrackId())
	span.End(nil)
	return connect.NewResponse(state), nil
}

func (h *handler) Pause(ctx context.Context, _ *connect.Request[relayv1.PauseRequest]) (*connect.Response[relayv1.PlayerState], error) {
	_, span := chassis.StartSpan(ctx, "relay.pause")
	state := h.player.Pause()
	span.End(nil)
	return connect.NewResponse(state), nil
}

func (h *handler) Seek(ctx context.Context, req *connect.Request[relayv1.SeekRequest]) (*connect.Response[relayv1.PlayerState], error) {
	_, span := chassis.StartSpan(ctx, "relay.seek")
	span.SetBusinessAttribute("position_ms", strconv.FormatInt(req.Msg.GetPositionMs(), 10))
	state := h.player.Seek(req.Msg.GetPositionMs())
	span.End(nil)
	return connect.NewResponse(state), nil
}

func (h *handler) SetVolume(ctx context.Context, req *connect.Request[relayv1.SetVolumeRequest]) (*connect.Response[relayv1.PlayerState], error) {
	_, span := chassis.StartSpan(ctx, "relay.set_volume")
	span.SetBusinessAttribute("volume_pct", strconv.Itoa(int(req.Msg.GetVolumePct())))
	state := h.player.SetVolume(req.Msg.GetVolumePct())
	span.End(nil)
	return connect.NewResponse(state), nil
}

func (h *handler) GetPlayerState(ctx context.Context, _ *connect.Request[relayv1.GetPlayerStateRequest]) (*connect.Response[relayv1.PlayerState], error) {
	_, span := chassis.StartSpan(ctx, "relay.get_player_state")
	state := h.player.State()
	span.End(nil)
	return connect.NewResponse(state), nil
}

func (h *handler) StreamAudioOut(ctx context.Context, _ *connect.Request[relayv1.StreamAudioOutRequest], stream *connect.ServerStream[relayv1.AudioChunk]) error {
	logger := h.logger.WithContext(ctx)
	var seq int64

	for {
		if ctx.Err() != nil {
			return nil
		}

		snap := h.player.Snapshot()
		if !snap.Playing || snap.TrackID == "" {
			select {
			case <-ctx.Done():
				return nil
			case <-h.player.Wake():
				continue
			}
		}

		track, err := h.model.GetTrack(ctx, snap.TrackID)
		if err != nil {
			logger.WithError(err).WithField("track_id", snap.TrackID).Error("stream_audio_out: track vanished, pausing")
			h.player.Pause()
			continue
		}

		if err := h.streamTrack(ctx, stream, track, snap, &seq); err != nil {
			return err
		}
		// streamTrack returning nil means either it hit a generation change (some other call
		// superseded this playback -- re-snapshot above and pick up the new state) or the file
		// ended on its own (Player.Finished already reset state) -- both cases just loop back.
	}
}

// streamTrack sends one track's bytes from snap.PositionMs onward, at real playback pace, until
// either the file ends or the player's generation moves past snap.Generation (a concurrent
// Pause/Seek/Play). Returning nil covers both outcomes; only a genuine I/O or send error is
// returned, which StreamAudioOut treats as fatal to the whole RPC.
func (h *handler) streamTrack(ctx context.Context, stream *connect.ServerStream[relayv1.AudioChunk], track *relayv1.Track, snap Snapshot, seq *int64) error {
	logger := h.logger.WithContext(ctx).WithField("track_id", track.GetId())

	f, err := os.Open(track.GetStoragePath())
	if err != nil {
		logger.WithError(err).Error("stream_audio_out: failed to open track file")
		h.player.Pause()
		return nil
	}
	defer f.Close()

	info, err := parseWAV(f)
	if err != nil {
		logger.WithError(err).Error("stream_audio_out: failed to parse track file")
		h.player.Pause()
		return nil
	}

	byteOffset := int64(float64(snap.PositionMs) / 1000.0 * float64(info.ByteRate))
	if byteOffset > info.DataSize {
		byteOffset = info.DataSize
	}
	if _, err := f.Seek(info.DataOffset+byteOffset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek track file: %w", err)
	}

	buf := make([]byte, audioChunkBytes)
	for {
		if ctx.Err() != nil {
			return nil
		}

		n, rerr := f.Read(buf)
		if n > 0 {
			chunk := &relayv1.AudioChunk{Pcm: append([]byte(nil), buf[:n]...), Sequence: *seq}
			if serr := stream.Send(chunk); serr != nil {
				return serr
			}
			*seq++

			deltaMs := int64(float64(n) / float64(info.ByteRate) * 1000.0)
			if !h.player.Advance(snap.Generation, deltaMs) {
				return nil // superseded by a concurrent Pause/Seek/Play -- caller re-snapshots
			}

			// Real-time pacing: send at roughly the rate the browser will consume it. Phase 3
			// does no client-side buffering of its own (see the plan's Phase 3 scope), so
			// blasting the whole file as fast as disk I/O allows would defeat "working transport
			// controls" -- Pause/Seek need to land against audio that hasn't already all arrived.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Duration(deltaMs) * time.Millisecond):
			}
		}

		if rerr == io.EOF {
			if serr := stream.Send(&relayv1.AudioChunk{Sequence: *seq, IsFinal: true}); serr != nil {
				return serr
			}
			h.player.Finished(snap.Generation)
			return nil
		}
		if rerr != nil {
			return fmt.Errorf("failed to read track file: %w", rerr)
		}
	}
}

// -- Recording + live transcription (Phase 4) --------------------------------------------------
//
// StartRecording/StreamAudioIn/AddMarker/StopRecording/WatchTranscript all operate on the single
// shared browser-input recording session (session.go) regardless of what input_device_id a
// request names -- see BrowserInputDeviceID's own comment. Diarization is batch-only (Phase 8):
// every segment persisted here has an empty speaker_id, permanently, not as a placeholder.

func (h *handler) StartRecording(ctx context.Context, req *connect.Request[relayv1.StartRecordingRequest]) (*connect.Response[relayv1.Recording], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.start_recording")
	span.SetBusinessAttribute("name", req.Msg.GetName())
	span.SetBusinessAttribute("input_device_id", req.Msg.GetInputDeviceId())

	if active := h.sessions.Active(); active != nil {
		err := fmt.Errorf("a recording is already in progress (recording_id=%s)", active.RecordingID)
		span.End(err)
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	rec, err := h.recordings.StartRecording(ctx, req.Msg.GetName())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to start recording")
		span.End(err)
		return nil, err
	}

	tmp, err := os.CreateTemp(h.storageDir, "recording-*.pcm")
	if err != nil {
		span.End(err)
		return nil, fmt.Errorf("failed to create temp recording file: %w", err)
	}
	if _, err := h.sessions.Start(rec.GetId(), tmp); err != nil {
		// Lost a race against a concurrent StartRecording between the check above and here.
		// Rare (Phase 4 has exactly one input device -- see BrowserInputDeviceID), but leaves an
		// orphaned LIVE db row with no session behind it if left alone; finish it immediately as
		// COMPLETE with zero duration instead of a permanently-LIVE ghost row.
		tmp.Close()
		os.Remove(tmp.Name())
		_, _ = h.recordings.FinishRecording(ctx, rec.GetId(), "", relayv1.RecordingStatus_RECORDING_STATUS_COMPLETE, 0, 0)
		span.End(err)
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	span.SetBusinessAttribute("recording_id", rec.GetId())
	span.End(nil)
	return connect.NewResponse(rec), nil
}

func (h *handler) StreamAudioIn(ctx context.Context, stream *connect.ClientStream[relayv1.AudioChunk]) (*connect.Response[relayv1.StreamAudioInResponse], error) {
	session := h.sessions.Active()
	if session == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no recording in progress -- call StartRecording first"))
	}

	ctx, span := chassis.StartSpan(ctx, "relay.stream_audio_in")
	span.SetBusinessAttribute("recording_id", session.RecordingID)

	var chunks int64
	for stream.Receive() {
		if data := stream.Msg().GetPcm(); len(data) > 0 {
			if err := session.Ingest(ctx, data, h.transcriber, h.recordings, h.events, h.logger); err != nil {
				// Unlike a single failed transcription window (logged internally, non-fatal),
				// this means the audio itself failed to persist -- serious enough to end the RPC
				// rather than silently keep dropping the recording's own bytes.
				h.logger.WithContext(ctx).WithError(err).WithField("recording_id", session.RecordingID).Error("stream_audio_in: failed to ingest chunk")
				span.End(err)
				return nil, err
			}
		}
		chunks++
	}
	if err := stream.Err(); err != nil {
		span.End(err)
		return nil, err
	}

	span.SetRuntimeAttribute("chunks_received", strconv.FormatInt(chunks, 10))
	span.End(nil)
	return connect.NewResponse(&relayv1.StreamAudioInResponse{RecordingId: session.RecordingID}), nil
}

// StreamAudioInChunk is what the Dioxus web client actually calls, once per captured buffer --
// see its own .proto doc for why StreamAudioIn's client-streaming shape doesn't work from a real
// browser. Same ingestion path as StreamAudioIn's own receive loop above, just one chunk per call.
func (h *handler) StreamAudioInChunk(ctx context.Context, req *connect.Request[relayv1.AudioChunk]) (*connect.Response[relayv1.StreamAudioInChunkResponse], error) {
	session := h.sessions.Active()
	if session == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no recording in progress -- call StartRecording first"))
	}

	if data := req.Msg.GetPcm(); len(data) > 0 {
		if err := session.Ingest(ctx, data, h.transcriber, h.recordings, h.events, h.logger); err != nil {
			h.logger.WithContext(ctx).WithError(err).WithField("recording_id", session.RecordingID).Error("stream_audio_in_chunk: failed to ingest chunk")
			return nil, err
		}
	}
	return connect.NewResponse(&relayv1.StreamAudioInChunkResponse{}), nil
}

func (h *handler) AddMarker(ctx context.Context, req *connect.Request[relayv1.AddMarkerRequest]) (*connect.Response[relayv1.Marker], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.add_marker")
	span.SetBusinessAttribute("recording_id", req.Msg.GetRecordingId())
	span.SetBusinessAttribute("label", req.Msg.GetLabel())

	session := h.sessions.Active()
	if session == nil || session.RecordingID != req.Msg.GetRecordingId() {
		// AddMarkerRequest carries no at_ms of its own (see the .proto) -- a marker's timestamp is
		// always "now, into the currently-live recording", which only makes sense while one is
		// actually live; there's nothing meaningful to stamp otherwise.
		err := fmt.Errorf("recording %q is not currently live -- markers can only be added while recording", req.Msg.GetRecordingId())
		span.End(err)
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	marker, err := h.recordings.AddMarker(ctx, req.Msg.GetRecordingId(), session.PositionMs(), req.Msg.GetLabel())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to add marker")
		span.End(err)
		return nil, err
	}
	span.End(nil)
	return connect.NewResponse(marker), nil
}

func (h *handler) StopRecording(ctx context.Context, req *connect.Request[relayv1.StopRecordingRequest]) (*connect.Response[relayv1.Recording], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.stop_recording")
	span.SetBusinessAttribute("recording_id", req.Msg.GetRecordingId())
	// No input_device_id here even though the Telemetry table lists it alongside StartRecording's:
	// StopRecordingRequest carries only recording_id (see the .proto) -- there's no field to read
	// one from, not an oversight matching StartRecording's own.

	session := h.sessions.Active()
	if session == nil || session.RecordingID != req.Msg.GetRecordingId() {
		err := fmt.Errorf("recording %q is not currently live", req.Msg.GetRecordingId())
		span.End(err)
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	// Flush transcribes whatever trailing audio hasn't hit a full window boundary yet -- without
	// this, up to ~3s at the end of every recording would be silently missing from the transcript
	// even though it's still present in the finalized audio file below.
	if err := session.Flush(ctx, h.transcriber, h.recordings, h.events, h.logger); err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to flush trailing transcript segment")
	}

	// Finalize (session.go) writes a real WAV header ahead of the session's accumulated raw PCM
	// at a fresh temp path; SaveUploadedTrack (model.go, the same Phase 3 code UploadTrack uses)
	// then parses that WAV, moves it into the library under its own track id, and inserts the row
	// -- recordingID set this time, unlike a plain library upload's "".
	tmpOutPath := filepath.Join(h.storageDir, "recording-final-"+uuid.NewString()+".wav")
	if _, err := session.Finalize(tmpOutPath); err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to finalize recording audio")
		span.End(err)
		return nil, err
	}

	// The recording's own name (set at StartRecording, editable only there -- see the Record page's
	// own Name field) becomes this track's title, so the Library list shows what the user actually
	// called it instead of every recording appearing as "Untitled". A lookup, not something carried
	// on RecordingSession itself -- StartRecording's name was already persisted to recordingRow and
	// nothing else here needs the full Recording, so a second row isn't worth a new session field.
	recordingName := ""
	if existing, err := h.recordings.GetRecording(ctx, req.Msg.GetRecordingId()); err == nil {
		recordingName = existing.GetName()
	}
	track, err := h.model.SaveUploadedTrack(ctx, tmpOutPath, &relayv1.AddTrackRequest{Title: recordingName}, req.Msg.GetRecordingId())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to save finalized recording's track")
		span.End(err)
		return nil, err
	}

	wordCount, avgConfidence := session.Stats()
	status := relayv1.RecordingStatus_RECORDING_STATUS_COMPLETE
	if wordCount > 0 && avgConfidence < lowConfidenceThreshold {
		status = relayv1.RecordingStatus_RECORDING_STATUS_LOW_CONFIDENCE
	}

	rec, err := h.recordings.FinishRecording(ctx, req.Msg.GetRecordingId(), track.GetId(), status, track.GetDurationMs(), wordCount)
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to finalize recording")
		span.End(err)
		return nil, err
	}
	h.sessions.End(session)

	// Phase 8's batch diarization pass, run synchronously so that by the time this RPC returns,
	// the archived transcript is already correctly split by speaker (see the plan's own Phase 8
	// deliverable: "once stopped, has its archived transcript correctly split"). Never fails
	// StopRecording itself -- the recording and its transcript are already safely finalized above;
	// diarization is a purely additive enrichment on top, and its own failure (most likely: no
	// pyannote.audio sidecar configured, or its gated model has no HuggingFace token -- see the
	// plan's Speech-to-text pipeline section) just leaves every segment's speaker_id "", exactly
	// how Phase 4 already left it.
	h.diarizeRecording(ctx, req.Msg.GetRecordingId(), track.GetStoragePath())
	if speakers, err := h.recordings.GetSpeakers(ctx, req.Msg.GetRecordingId()); err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to reload speakers after diarization")
	} else {
		rec.Speakers = speakers
	}

	span.SetBusinessAttribute("track_id", track.GetId())
	span.SetRuntimeAttribute("word_count", strconv.Itoa(int(wordCount)))
	span.SetRuntimeAttribute("speaker_count", strconv.Itoa(len(rec.GetSpeakers())))
	span.End(nil)
	return connect.NewResponse(rec), nil
}

// diarizeRecording runs Phase 8's batch diarization pass over a just-finalized recording's audio
// and relabels its already-persisted transcript segments. See this file's own package doc comment
// above StopRecording for why a failure here is always swallowed (logged, not propagated).
func (h *handler) diarizeRecording(ctx context.Context, recordingID, wavPath string) {
	logger := h.logger.WithContext(ctx).WithField("recording_id", recordingID)

	wavBytes, err := os.ReadFile(wavPath)
	if err != nil {
		logger.WithError(err).Error("diarization: failed to read finalized recording audio")
		return
	}

	diarized, err := h.diarizer.Diarize(ctx, wavBytes)
	if err != nil {
		logger.WithError(err).Error("diarization: pyannote.audio call failed -- transcript stays unlabeled")
		return
	}
	if len(diarized) == 0 {
		return
	}

	// Map pyannote's own raw labels ("SPEAKER_00", ...) to persisted Speakers with sequential
	// display labels, in the order each first appears in the recording -- not pyannote's own label
	// numbering, which has no guaranteed correspondence to speaking order.
	speakerIDByRawLabel := make(map[string]string)
	var order []string
	for _, seg := range diarized {
		if _, seen := speakerIDByRawLabel[seg.SpeakerLabel]; !seen {
			speakerIDByRawLabel[seg.SpeakerLabel] = "" // reserve the slot; filled in below
			order = append(order, seg.SpeakerLabel)
		}
	}
	for i, rawLabel := range order {
		speaker, err := h.recordings.CreateSpeaker(ctx, recordingID, fmt.Sprintf("Speaker %d", i+1))
		if err != nil {
			logger.WithError(err).WithField("raw_label", rawLabel).Error("diarization: failed to create speaker")
			continue
		}
		speakerIDByRawLabel[rawLabel] = speaker.GetId()
	}

	segments, err := h.recordings.GetTranscript(ctx, recordingID)
	if err != nil {
		logger.WithError(err).Error("diarization: failed to load transcript segments to relabel")
		return
	}

	for _, ts := range segments {
		bestSpeakerID, bestOverlap := "", int64(0)
		for _, d := range diarized {
			speakerID := speakerIDByRawLabel[d.SpeakerLabel]
			if speakerID == "" {
				continue // that speaker failed to persist above -- can't relabel to it
			}
			if overlap := overlapMs(ts.GetStartMs(), ts.GetEndMs(), d.StartMs, d.EndMs); overlap > bestOverlap {
				bestOverlap, bestSpeakerID = overlap, speakerID
			}
		}
		if bestSpeakerID == "" {
			continue // no diarized span overlapped this segment -- leave it unlabeled
		}
		if err := h.recordings.SetTranscriptSegmentSpeaker(ctx, ts.GetId(), bestSpeakerID); err != nil {
			logger.WithError(err).WithField("segment_id", ts.GetId()).Error("diarization: failed to relabel segment")
		}
	}

	logger.WithField("speaker_count", len(order)).Info("diarization: relabeled transcript")
}

func (h *handler) WatchTranscript(ctx context.Context, req *connect.Request[relayv1.WatchTranscriptRequest], stream *connect.ServerStream[relayv1.TranscriptSegment]) error {
	session := h.sessions.Active()
	if session == nil || session.RecordingID != req.Msg.GetRecordingId() {
		// Nothing more will ever arrive for a recording that isn't currently live -- Phase 5's
		// GetTranscript (archive) is how a finished recording's segments get read back, not this
		// RPC. Returning nil (rather than an error) closes the stream cleanly with zero messages,
		// which is a legitimate outcome here, not a failure.
		return nil
	}

	ch, unsubscribe := session.Subscribe()
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return nil
		case seg, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(seg); err != nil {
				return err
			}
		}
	}
}

// -- Archive + search (Phase 5) ------------------------------------------------------------------

func (h *handler) ListRecordings(ctx context.Context, _ *connect.Request[relayv1.ListRecordingsRequest]) (*connect.Response[relayv1.ListRecordingsResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.list_recordings")

	recs, err := h.recordings.ListRecordings(ctx)
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to list recordings")
		span.End(err)
		return nil, err
	}
	span.SetRuntimeAttribute("recording_count", strconv.Itoa(len(recs)))
	span.End(nil)
	return connect.NewResponse(&relayv1.ListRecordingsResponse{Recordings: recs}), nil
}

func (h *handler) GetRecording(ctx context.Context, req *connect.Request[relayv1.GetRecordingRequest]) (*connect.Response[relayv1.Recording], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.get_recording")
	span.SetBusinessAttribute("recording_id", req.Msg.GetId())

	rec, err := h.recordings.GetRecording(ctx, req.Msg.GetId())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).WithField("recording_id", req.Msg.GetId()).Error("failed to get recording")
		span.End(err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	span.End(nil)
	return connect.NewResponse(rec), nil
}

func (h *handler) GetTranscript(ctx context.Context, req *connect.Request[relayv1.GetTranscriptRequest]) (*connect.Response[relayv1.GetTranscriptResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.get_transcript")
	span.SetBusinessAttribute("recording_id", req.Msg.GetRecordingId())

	segs, err := h.recordings.GetTranscript(ctx, req.Msg.GetRecordingId())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).WithField("recording_id", req.Msg.GetRecordingId()).Error("failed to get transcript")
		span.End(err)
		return nil, err
	}
	span.SetRuntimeAttribute("segment_count", strconv.Itoa(len(segs)))
	span.End(nil)
	return connect.NewResponse(&relayv1.GetTranscriptResponse{Segments: segs}), nil
}

func (h *handler) SearchTranscripts(ctx context.Context, req *connect.Request[relayv1.SearchTranscriptsRequest]) (*connect.Response[relayv1.SearchTranscriptsResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.search_transcripts")
	span.SetBusinessAttribute("query", req.Msg.GetQuery())

	hits, err := h.recordings.SearchTranscripts(ctx, req.Msg.GetQuery())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to search transcripts")
		span.End(err)
		return nil, err
	}
	span.SetBusinessAttribute("result_count", strconv.Itoa(len(hits)))
	span.End(nil)
	return connect.NewResponse(&relayv1.SearchTranscriptsResponse{Hits: hits}), nil
}

// -- Action items -> Lineman (Phase 6) -------------------------------------------------------------
//
// CreateActionItem is an addition beyond the plan's own original RPC Interface sketch -- see its
// .proto doc for why (ListActionItems/CreateLinemanTasks assumed ActionItems already existed with
// no RPC that could ever create one).

func (h *handler) CreateActionItem(ctx context.Context, req *connect.Request[relayv1.CreateActionItemRequest]) (*connect.Response[relayv1.ActionItem], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.create_action_item")
	span.SetBusinessAttribute("recording_id", req.Msg.GetRecordingId())
	span.SetBusinessAttribute("text", req.Msg.GetText())

	item, err := h.recordings.CreateActionItem(ctx, req.Msg)
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to create action item")
		span.End(err)
		return nil, err
	}
	span.SetBusinessAttribute("action_item_id", item.GetId())
	span.End(nil)
	return connect.NewResponse(item), nil
}

func (h *handler) ListActionItems(ctx context.Context, req *connect.Request[relayv1.ListActionItemsRequest]) (*connect.Response[relayv1.ListActionItemsResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.list_action_items")
	span.SetBusinessAttribute("recording_id", req.Msg.GetRecordingId())

	items, err := h.recordings.ListActionItems(ctx, req.Msg.GetRecordingId())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to list action items")
		span.End(err)
		return nil, err
	}
	span.SetRuntimeAttribute("action_item_count", strconv.Itoa(len(items)))
	span.End(nil)
	return connect.NewResponse(&relayv1.ListActionItemsResponse{ActionItems: items}), nil
}

// CreateLinemanTasks calls Lineman's own real CreateTask RPC once per requested action item,
// standalone (no objective_id -- Relay has no notion of a Lineman Objective to attach to), then
// records the resulting task id back onto each action item so a second promotion attempt is
// visible as already-done rather than silently creating a duplicate task. Fails the whole call on
// the first Lineman error rather than a partial-success/partial-failure result: the wire shape
// (CreateLinemanTasksResponse{lineman_task_ids}) has no way to report per-item failure, and a
// half-created batch of tasks with no indication which action items got one would be worse than
// requiring the caller to retry the whole selection.
func (h *handler) CreateLinemanTasks(ctx context.Context, req *connect.Request[relayv1.CreateLinemanTasksRequest]) (*connect.Response[relayv1.CreateLinemanTasksResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.create_lineman_tasks")

	items, err := h.recordings.GetActionItems(ctx, req.Msg.GetActionItemIds())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to load action items for promotion")
		span.End(err)
		return nil, err
	}
	if len(items) == 0 {
		span.End(nil)
		return connect.NewResponse(&relayv1.CreateLinemanTasksResponse{}), nil
	}
	span.SetBusinessAttribute("recording_id", items[0].GetRecordingId())

	taskIDs := make([]string, 0, len(items))
	for _, item := range items {
		details := fmt.Sprintf("From Relay recording %s", item.GetRecordingId())
		if item.GetSpeakerLabel() != "" {
			details += fmt.Sprintf(" — %s", item.GetSpeakerLabel())
		}
		if item.GetAtMs() > 0 {
			details += fmt.Sprintf(" at %s", formatMs(item.GetAtMs()))
		}

		taskResp, err := h.lineman.CreateTask(ctx, connect.NewRequest(&linemanv1.CreateTaskRequest{
			Name:    item.GetText(),
			Details: details,
		}))
		if err != nil {
			h.logger.WithContext(ctx).WithError(err).WithField("action_item_id", item.GetId()).Error("failed to create Lineman task")
			span.End(err)
			return nil, fmt.Errorf("failed to create Lineman task for action item %q: %w", item.GetId(), err)
		}

		taskID := taskResp.Msg.GetId()
		if err := h.recordings.SetLinemanTaskID(ctx, item.GetId(), taskID); err != nil {
			// The Lineman task was created successfully -- losing track of the link is a lesser
			// failure than losing the task itself, so log and keep going rather than erroring the
			// whole batch over a bookkeeping write.
			h.logger.WithContext(ctx).WithError(err).WithField("action_item_id", item.GetId()).Error("failed to record lineman_task_id")
		}
		taskIDs = append(taskIDs, taskID)
	}

	span.SetBusinessAttribute("task_count", strconv.Itoa(len(taskIDs)))
	span.SetBusinessAttribute("lineman_task_ids", strings.Join(taskIDs, ","))
	span.End(nil)
	return connect.NewResponse(&relayv1.CreateLinemanTasksResponse{LinemanTaskIds: taskIDs}), nil
}

// formatMs renders a millisecond offset as m:ss for a Lineman task's own details text --
// "at 1:23", not a raw millisecond count a human has to do math on.
func formatMs(ms int64) string {
	totalSeconds := ms / 1000
	return fmt.Sprintf("%d:%02d", totalSeconds/60, totalSeconds%60)
}

// -- Speaker diarization (Phase 8) -----------------------------------------------------------------
//
// RenameSpeaker is the only Phase 8 RPC beyond StopRecording's own diarization trigger above --
// Speaker rows are created by diarizeRecording, never directly by a client.

func (h *handler) RenameSpeaker(ctx context.Context, req *connect.Request[relayv1.RenameSpeakerRequest]) (*connect.Response[relayv1.Speaker], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.rename_speaker")
	span.SetBusinessAttribute("speaker_id", req.Msg.GetSpeakerId())
	span.SetBusinessAttribute("label", req.Msg.GetLabel())

	speaker, err := h.recordings.RenameSpeaker(ctx, req.Msg.GetSpeakerId(), req.Msg.GetLabel())
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to rename speaker")
		span.End(err)
		return nil, err
	}
	span.End(nil)
	return connect.NewResponse(speaker), nil
}

// -- Devices (Phase 9) -----------------------------------------------------------------------------
//
// Listing only this phase: real hardware enumeration (devices.go/devices_portaudio.go), always
// merged with the two virtual browser devices every phase since Phase 3 has relied on. Routing
// Play/StartRecording to a real native device_id is a disclosed follow-up, not built here -- see
// the implementation plan's own Phase 9 completion note for why (it needs a Player generalized to
// one per output device, and a native capture pump feeding the existing recording pipeline,
// neither of which this phase touches).

func (h *handler) ListDevices(ctx context.Context, _ *connect.Request[relayv1.ListDevicesRequest]) (*connect.Response[relayv1.ListDevicesResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "relay.list_devices")

	native, err := h.devices.ListDevices()
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to enumerate native devices")
		span.End(err)
		return nil, err
	}

	devices := []*relayv1.Device{
		{Id: BrowserInputDeviceID, Name: "MacBook mic · browser", Kind: relayv1.DeviceKind_DEVICE_KIND_INPUT},
		{Id: BrowserOutputDeviceID, Name: "Stream to browser", Kind: relayv1.DeviceKind_DEVICE_KIND_OUTPUT},
	}
	devices = append(devices, native...)

	span.SetRuntimeAttribute("device_count", strconv.Itoa(len(devices)))
	span.End(nil)
	return connect.NewResponse(&relayv1.ListDevicesResponse{Devices: devices}), nil
}
