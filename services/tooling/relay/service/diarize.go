package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// DiarizationSegment is one speaker-attributed span pyannote.audio's own pipeline found, in its
// own raw label space (e.g. "SPEAKER_00") -- service-local, not the wire type: a raw diarization
// result has no persisted Speaker id yet, and its labels aren't the "Speaker 1"/"Speaker 2" display
// labels users see until relabelRecordingSpeakers (recording.go) maps them.
type DiarizationSegment struct {
	StartMs      int64
	EndMs        int64
	SpeakerLabel string
}

// Diarizer runs a complete recording's audio through a diarization pass. Unlike Transcriber
// (whisper.cpp ships a ready-made server), there is no off-the-shelf pyannote.audio server -- the
// HTTP contract below (POST /diarize, multipart WAV, JSON array response) is this package's own
// design, implemented by pyannote-wrapper/server.py (see that file, and the implementation plan's
// Speech-to-text pipeline section for the reasoning). Kept as an interface for the same reason
// Transcriber is: the batch relabeling logic (recording.go) is what's worth testing without
// needing a real pyannote.audio process -- and, more than for whisper.cpp, without needing the
// HuggingFace-gated model pyannote's own pretrained pipeline requires (see BuildManifest's own
// Decisions-section caveat) -- to exercise.
type Diarizer interface {
	Diarize(ctx context.Context, wavBytes []byte) ([]DiarizationSegment, error)
}

type pyannoteDiarizer struct {
	address    string // e.g. http://127.0.0.1:9310 -- pyannote-wrapper/server.py
	httpClient *http.Client
}

func NewPyannoteDiarizer(address string) Diarizer {
	return &pyannoteDiarizer{
		address: address,
		// A full-recording diarization pass runs pyannote's pipeline over the WHOLE file at once
		// (batch, not streaming -- see the plan's own "batch-only, by design" decision), which can
		// take meaningfully longer than whisper.cpp's per-window transcription calls on CPU;
		// generous but still bounded so a wedged sidecar can't hang StopRecording forever.
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// diarizationSegmentJSON mirrors pyannote-wrapper/server.py's own /diarize response shape exactly:
// a plain JSON array of {start_ms, end_ms, speaker_label} objects -- no envelope, since there's
// nothing else to say about a diarization result.
type diarizationSegmentJSON struct {
	StartMs      int64  `json:"start_ms"`
	EndMs        int64  `json:"end_ms"`
	SpeakerLabel string `json:"speaker_label"`
}

func (d *pyannoteDiarizer) Diarize(ctx context.Context, wavBytes []byte) ([]DiarizationSegment, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "recording.wav")
	if err != nil {
		return nil, fmt.Errorf("failed to build multipart request: %w", err)
	}
	if _, err := part.Write(wavBytes); err != nil {
		return nil, fmt.Errorf("failed to write audio into multipart request: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.address+"/diarize", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach pyannote.audio sidecar at %s: %w", d.address, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read pyannote.audio response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The wrapper's own most likely failure mode -- no HuggingFace token configured, so the
		// gated pretrained pipeline never loaded -- returns a clear message in the body here
		// rather than a bare HTTP status; surfaced as-is so it reaches this service's own logs.
		return nil, fmt.Errorf("pyannote.audio returned %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed []diarizationSegmentJSON
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse pyannote.audio response: %w", err)
	}

	segments := make([]DiarizationSegment, len(parsed))
	for i, s := range parsed {
		segments[i] = DiarizationSegment{StartMs: s.StartMs, EndMs: s.EndMs, SpeakerLabel: s.SpeakerLabel}
	}
	return segments, nil
}

// overlapMs is the length, in ms, that [aStart, aEnd) and [bStart, bEnd) share -- 0 if they don't
// overlap at all. Used by (*handler).diarizeRecording (rpc.go) to decide which diarized speaker
// span a given transcript segment belongs to.
func overlapMs(aStart, aEnd, bStart, bEnd int64) int64 {
	start := aStart
	if bStart > start {
		start = bStart
	}
	end := aEnd
	if bEnd < end {
		end = bEnd
	}
	if end <= start {
		return 0
	}
	return end - start
}
