package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"time"
)

// TranscriptSegment is one segment whisper.cpp returned for a single transcription call --
// service-local, not the wire type (relayv1.TranscriptSegment), since a raw whisper.cpp result
// has no recording_id/id yet and its timestamps are relative to whatever audio buffer was sent,
// not the recording's own timeline; session.go adds both before this ever reaches a caller.
type TranscriptSegment struct {
	StartMs    int64
	EndMs      int64
	Text       string
	Confidence float32
}

// Transcriber turns a WAV byte buffer into finalized segments. The only implementation is
// whisperTranscriber (below), calling the whisper.cpp server sidecar described in the
// implementation plan's Speech-to-text pipeline section -- kept as an interface so session.go's
// windowing logic (which is the part worth testing) doesn't need a real whisper.cpp process to
// exercise.
type Transcriber interface {
	Transcribe(ctx context.Context, wavBytes []byte) ([]TranscriptSegment, error)
}

type whisperTranscriber struct {
	address    string // e.g. http://127.0.0.1:9309 -- whisper.cpp's own `server` example
	httpClient *http.Client
}

func NewWhisperTranscriber(address string) Transcriber {
	return &whisperTranscriber{
		address: address,
		// whisper.cpp's own inference call can legitimately take longer than a typical RPC
		// timeout for a several-second audio window on CPU; generous but bounded so a wedged
		// sidecar can't hang a recording session forever.
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// whisperVerboseJSON mirrors the subset of whisper.cpp server's response_format=verbose_json
// shape (OpenAI Whisper API-compatible) this needs -- confirmed live against a running
// whisper-server: POST /inference with a multipart "file" + response_format=verbose_json returns
// {..., "segments": [{"start": <seconds float>, "end": <seconds float>, "text": ...,
// "avg_logprob": <negative log-probability>, ...}]}. avg_logprob, not a 0-1 confidence, is the
// only per-segment confidence signal the server returns -- converted below.
type whisperVerboseJSON struct {
	Segments []struct {
		Start      float64 `json:"start"`
		End        float64 `json:"end"`
		Text       string  `json:"text"`
		AvgLogprob float64 `json:"avg_logprob"`
	} `json:"segments"`
}

func (t *whisperTranscriber) Transcribe(ctx context.Context, wavBytes []byte) ([]TranscriptSegment, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "audio.wav")
	if err != nil {
		return nil, fmt.Errorf("failed to build multipart request: %w", err)
	}
	if _, err := part.Write(wavBytes); err != nil {
		return nil, fmt.Errorf("failed to write audio into multipart request: %w", err)
	}
	if err := mw.WriteField("response_format", "verbose_json"); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.address+"/inference", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach whisper.cpp sidecar at %s: %w", t.address, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read whisper.cpp response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("whisper.cpp returned %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed whisperVerboseJSON
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse whisper.cpp response: %w", err)
	}

	segments := make([]TranscriptSegment, 0, len(parsed.Segments))
	for _, s := range parsed.Segments {
		segments = append(segments, TranscriptSegment{
			StartMs: int64(s.Start * 1000),
			EndMs:   int64(s.End * 1000),
			Text:    s.Text,
			// avg_logprob is a mean log-probability (<=0, closer to 0 is more confident) --
			// exponentiating gives a 0-1 pseudo-confidence, a standard (if approximate)
			// conversion; there is no direct 0-1 confidence field in the server's own response.
			Confidence: float32(math.Exp(s.AvgLogprob)),
		})
	}
	return segments, nil
}
