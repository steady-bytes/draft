package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	relayv1 "github.com/steady-bytes/draft/api/tooling/relay/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/google/uuid"
)

const (
	// BrowserInputDeviceID is Phase 4's one and only input device -- "MacBook mic · browser" in
	// mockups/pages/relay-record.html, fed by the browser's MediaRecorder/Web Audio capture (no
	// native PortAudio hardware capture until Phase 9). Exactly one input device means at most one
	// live recording session on this instance at a time -- the same "one shared thing, Phase N
	// only has one of it" simplification Phase 3 already applies via BrowserOutputDeviceID.
	//
	// This is also *why* StreamAudioIn's own recording_id ambiguity resolves cleanly:
	// api/tooling/relay/v1/service.proto's AudioChunk carries no recording_id field, so a
	// StreamAudioIn call has no way to name which recording it's for except "whichever one is
	// currently live" -- true by construction as long as at most one can be.
	//
	// Distinct from BrowserOutputDeviceID (player.go) -- see that constant's own doc for why they
	// used to collide on the literal string "browser" until Phase 9's ListDevices exposed it.
	BrowserInputDeviceID = "browser-in"

	// Phase 4's required capture format -- both whisper.cpp's own requirement and the format
	// every PCM buffer/WAV file this package writes during recording is assumed to be in. The
	// Phase 10 web client will need to resample the browser's own capture (typically 44.1/48kHz)
	// down to this before sending StreamAudioIn chunks; not negotiated per-stream today.
	whisperSampleRateHz  = 16000
	whisperChannels      = 1
	whisperBitsPerSample = 16
)

var whisperBytesPerSecond int64 = whisperSampleRateHz * whisperChannels * whisperBitsPerSample / 8

// transcriptionWindowBytes is how much newly-arrived audio triggers one whisper.cpp call -- a
// fixed, non-overlapping ~3s window, not a sliding/growing one: each window is transcribed and
// finalized independently and immediately (transcribeWindow below), so whisper.cpp's own per-call
// cost never grows with how long the recording has been running. The real tradeoff this makes,
// disclosed rather than silent: a sentence split across a window boundary can come out
// mis-transcribed at the seam, and captions arrive in ~3s batches, not word-by-word -- see the
// implementation plan's Phase 4 completion note.
var transcriptionWindowBytes = whisperBytesPerSecond * 3

func bytesToMs(n int64) int64 {
	return int64(float64(n) / float64(whisperBytesPerSecond) * 1000.0)
}

// RecordingSession is the in-memory state for one currently-live recording: raw PCM ingested so
// far (persisted incrementally to a temp file, not held in memory -- a recording can run far
// longer than a Phase 3 upload), plus whoever's currently watching its transcript live. Exactly
// one of these exists at a time on a given Relay instance (see BrowserInputDeviceID);
// SessionManager below owns that single slot.
type RecordingSession struct {
	RecordingID string

	mu              sync.Mutex
	file            *os.File // append-only raw PCM, no WAV header yet -- StopRecording adds one
	totalBytes      int64
	untranscribed   int64 // bytes written since the last transcription window fired
	subscribers     map[string]chan *relayv1.TranscriptSegment
	wordCount       int32
	confidenceSum   float64
	confidenceCount int32
}

func newRecordingSession(recordingID string, file *os.File) *RecordingSession {
	return &RecordingSession{
		RecordingID: recordingID,
		file:        file,
		subscribers: make(map[string]chan *relayv1.TranscriptSegment),
	}
}

// PositionMs is how far into the recording totalBytes reaches -- the mockup's own running
// recording-length readout, and what StopRecording reports as the finished Track's duration.
func (s *RecordingSession) PositionMs() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytesToMs(s.totalBytes)
}

// Stats returns the running word count and average per-segment confidence across every finalized
// segment so far -- StopRecording uses these to fill in Recording.word_count and to decide between
// RECORDING_STATUS_COMPLETE and RECORDING_STATUS_LOW_CONFIDENCE. avgConfidence is 0 (not NaN) when
// no segments were ever produced (e.g. a recording stopped almost immediately, or pure silence).
func (s *RecordingSession) Stats() (wordCount int32, avgConfidence float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.confidenceCount == 0 {
		return s.wordCount, 0
	}
	return s.wordCount, s.confidenceSum / float64(s.confidenceCount)
}

// Subscribe registers a live listener for this session's finalized segments (WatchTranscript,
// rpc.go). The caller must run the returned unsubscribe func once its stream ends.
func (s *RecordingSession) Subscribe() (<-chan *relayv1.TranscriptSegment, func()) {
	id := uuid.NewString()
	ch := make(chan *relayv1.TranscriptSegment, 16)
	s.mu.Lock()
	s.subscribers[id] = ch
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

func (s *RecordingSession) publish(seg *relayv1.TranscriptSegment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- seg:
		default:
			// A slow WatchTranscript subscriber drops a caption rather than blocking ingestion for
			// everyone else -- the same non-blocking-per-subscriber shape as Catalyst's own
			// fan-out fix (services/core/catalyst/broker), applied here to live transcript push.
		}
	}
}

// Ingest writes newly-arrived PCM bytes to disk and, once a full transcriptionWindowBytes worth of
// new audio has accumulated, transcribes that window and persists+publishes its segments. Runs
// synchronously on StreamAudioIn's own receive loop (rpc.go) -- whisper.cpp's tiny-model call is
// fast enough (confirmed live: sub-second for a 3s window on this machine's Metal backend) that
// this doesn't meaningfully stall ingestion of the next chunk.
func (s *RecordingSession) Ingest(ctx context.Context, pcm []byte, transcriber Transcriber, model RecordingModel, events EventPublisher, logger chassis.Logger) error {
	s.mu.Lock()
	if _, err := s.file.Write(pcm); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to persist audio chunk: %w", err)
	}
	s.totalBytes += int64(len(pcm))
	s.untranscribed += int64(len(pcm))

	var windowStartByte, windowEndByte int64
	ready := s.untranscribed >= transcriptionWindowBytes
	if ready {
		windowEndByte = s.totalBytes
		windowStartByte = windowEndByte - s.untranscribed
		s.untranscribed = 0
	}
	s.mu.Unlock()

	if !ready {
		return nil
	}
	return s.transcribeWindow(ctx, windowStartByte, windowEndByte, transcriber, model, events, logger)
}

// transcribeWindow reads exactly [startByte, endByte) of this session's persisted raw PCM, wraps
// it as a standalone WAV buffer, sends it to whisper.cpp, and persists+publishes whatever segments
// come back with their timestamps shifted from "seconds into this window" to "milliseconds into
// the whole recording" -- the coordinate space TranscriptSegment.start_ms/end_ms and every
// subscriber actually expect.
func (s *RecordingSession) transcribeWindow(ctx context.Context, startByte, endByte int64, transcriber Transcriber, model RecordingModel, events EventPublisher, logger chassis.Logger) error {
	pcm := make([]byte, endByte-startByte)
	if _, err := s.file.ReadAt(pcm, startByte); err != nil {
		return fmt.Errorf("failed to read back transcription window: %w", err)
	}

	var wavBuf bytes.Buffer
	if err := writeWAVHeader(&wavBuf, int64(len(pcm)), whisperSampleRateHz, whisperChannels, whisperBitsPerSample); err != nil {
		return err
	}
	wavBuf.Write(pcm)

	segments, err := transcriber.Transcribe(ctx, wavBuf.Bytes())
	if err != nil {
		// A single failed window (sidecar hiccup, timeout) doesn't end the recording -- log and
		// keep ingesting; that window's speech is simply missing from the transcript, the same
		// tradeoff a dropped live caption anywhere would be.
		logger.WithContext(ctx).WithError(err).WithField("recording_id", s.RecordingID).Error("stream_audio_in: transcription window failed")
		return nil
	}

	windowOffsetMs := bytesToMs(startByte)
	for _, seg := range segments {
		if seg.Text == "" {
			continue
		}
		seg.StartMs += windowOffsetMs
		seg.EndMs += windowOffsetMs

		proto, err := model.InsertTranscriptSegment(ctx, s.RecordingID, seg)
		if err != nil {
			logger.WithContext(ctx).WithError(err).WithField("recording_id", s.RecordingID).Error("stream_audio_in: failed to persist transcript segment")
			continue
		}

		s.mu.Lock()
		s.wordCount += int32(len(strings.Fields(seg.Text)))
		s.confidenceSum += float64(seg.Confidence)
		s.confidenceCount++
		s.mu.Unlock()

		s.publish(proto)
		events.PublishTranscriptSegment(ctx, proto)
	}
	return nil
}

// Flush transcribes whatever audio has accumulated since the last window fired, even if it's less
// than a full transcriptionWindowBytes. StopRecording calls this once, before Finalize, so the
// tail end of a recording (anything shorter than one window's worth since the last boundary)
// isn't silently missing from the transcript -- a real gap Ingest's own fixed-window trigger
// leaves otherwise. A no-op if nothing is pending.
func (s *RecordingSession) Flush(ctx context.Context, transcriber Transcriber, model RecordingModel, events EventPublisher, logger chassis.Logger) error {
	s.mu.Lock()
	if s.untranscribed == 0 {
		s.mu.Unlock()
		return nil
	}
	windowEndByte := s.totalBytes
	windowStartByte := windowEndByte - s.untranscribed
	s.untranscribed = 0
	s.mu.Unlock()

	return s.transcribeWindow(ctx, windowStartByte, windowEndByte, transcriber, model, events, logger)
}

// Finalize writes a proper WAV header ahead of this session's accumulated raw PCM at finalPath,
// leaving the original temp file (headerless) untouched at its own path -- StopRecording (rpc.go)
// removes the temp file itself once this returns successfully. Returns the total PCM byte count
// written (== s.totalBytes) so the caller can compute the finished Track's duration/size without
// re-deriving it from the file a second time.
func (s *RecordingSession) Finalize(finalPath string) (sizeBytes int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out, err := os.Create(finalPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create finalized recording file: %w", err)
	}
	defer out.Close()

	if err := writeWAVHeader(out, s.totalBytes, whisperSampleRateHz, whisperChannels, whisperBitsPerSample); err != nil {
		return 0, err
	}
	if _, err := s.file.Seek(0, 0); err != nil {
		return 0, fmt.Errorf("failed to rewind recording's temp file: %w", err)
	}
	if _, err := io.Copy(out, s.file); err != nil {
		return 0, fmt.Errorf("failed to copy recorded audio into place: %w", err)
	}
	return s.totalBytes, nil
}

// SessionManager owns the single active RecordingSession slot -- see BrowserInputDeviceID's own
// comment for why Phase 4 only ever has one.
type SessionManager struct {
	mu      sync.Mutex
	current *RecordingSession
}

func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

// Start begins a new session, failing if one is already live -- StartRecording's own
// "one recording at a time" rule (rpc.go).
func (m *SessionManager) Start(recordingID string, file *os.File) (*RecordingSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil {
		return nil, fmt.Errorf("a recording is already in progress (recording_id=%s)", m.current.RecordingID)
	}
	s := newRecordingSession(recordingID, file)
	m.current = s
	return s, nil
}

// Active returns the current live session, or nil if none is running.
func (m *SessionManager) Active() *RecordingSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// End clears the active slot if it's still the given session (a no-op if something else already
// replaced it, which shouldn't happen given Start's own exclusivity check, but guards against a
// double-End race regardless).
func (m *SessionManager) End(s *RecordingSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == s {
		m.current = nil
	}
}
