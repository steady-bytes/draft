package service

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// wavInfo is everything StreamAudioOut and SaveUploadedTrack need out of a WAV file's header: the
// PCM format (to compute duration and byte-accurate seek offsets) and where the raw sample data
// actually starts.
type wavInfo struct {
	SampleRate    uint32
	Channels      uint16
	BitsPerSample uint16
	ByteRate      uint32 // bytes/sec of PCM audio = SampleRate * Channels * BitsPerSample/8
	DataOffset    int64  // byte offset of the "data" chunk's payload within the file
	DataSize      int64  // length of the PCM payload in bytes
}

// parseWAV reads a canonical RIFF/WAVE header from r and returns enough to compute duration and to
// seek to an arbitrary playback position. It handles only the common case -- one "fmt " chunk
// before one "data" chunk, PCM or IEEE-float samples -- deliberately, not a general RIFF parser:
// Phase 3's own scope is WAV-only uploads/playback (see the implementation plan's Phase 3 section),
// with MP3/FLAC/AAC decoding an explicitly disclosed follow-up rather than something silently
// half-supported here.
func parseWAV(r io.Reader) (*wavInfo, error) {
	var riffHeader [12]byte
	if _, err := io.ReadFull(r, riffHeader[:]); err != nil {
		return nil, fmt.Errorf("failed to read RIFF header: %w", err)
	}
	if string(riffHeader[0:4]) != "RIFF" || string(riffHeader[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}

	info := &wavInfo{}
	offset := int64(12)
	haveFmt := false

	for {
		var chunkHeader [8]byte
		if _, err := io.ReadFull(r, chunkHeader[:]); err != nil {
			return nil, fmt.Errorf("failed to read chunk header at offset %d: %w", offset, err)
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))
		offset += 8

		switch chunkID {
		case "fmt ":
			body := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, body); err != nil {
				return nil, fmt.Errorf("failed to read fmt chunk: %w", err)
			}
			if len(body) < 16 {
				return nil, errors.New("fmt chunk shorter than 16 bytes")
			}
			info.Channels = binary.LittleEndian.Uint16(body[2:4])
			info.SampleRate = binary.LittleEndian.Uint32(body[4:8])
			info.BitsPerSample = binary.LittleEndian.Uint16(body[14:16])
			haveFmt = true

		case "data":
			if !haveFmt {
				return nil, errors.New("data chunk arrived before fmt chunk")
			}
			if info.SampleRate == 0 || info.Channels == 0 || info.BitsPerSample == 0 {
				return nil, errors.New("fmt chunk had a zero sample_rate/channels/bits_per_sample")
			}
			info.DataOffset = offset
			info.DataSize = chunkSize
			info.ByteRate = info.SampleRate * uint32(info.Channels) * uint32(info.BitsPerSample) / 8
			return info, nil

		default:
			if _, err := io.CopyN(io.Discard, r, chunkSize); err != nil {
				return nil, fmt.Errorf("failed to skip %q chunk: %w", chunkID, err)
			}
		}

		offset += chunkSize
		if chunkSize%2 == 1 {
			// RIFF chunks are word-aligned; an odd-sized chunk is followed by one pad byte.
			if _, err := io.CopyN(io.Discard, r, 1); err != nil {
				return nil, fmt.Errorf("failed to skip chunk pad byte: %w", err)
			}
			offset++
		}
	}
}

func (w *wavInfo) durationMs() int64 {
	if w.ByteRate == 0 {
		return 0
	}
	return int64(float64(w.DataSize) / float64(w.ByteRate) * 1000.0)
}

func (w *wavInfo) format() string {
	return fmt.Sprintf("WAV %dHz/%d-bit", w.SampleRate, w.BitsPerSample)
}

// writeWAVHeader writes a canonical 44-byte PCM RIFF/WAVE header for dataSize bytes of raw PCM
// that follow it. This is parseWAV's inverse, used to turn the raw PCM16LE bytes StreamAudioIn
// receives (see api/tooling/relay/v1/service.proto's AudioChunk doc) into a real WAV file/byte
// buffer -- both for finalizing a Recording's Track (session.go) and for wrapping a transcription
// window before handing it to whisper.cpp, which requires a proper WAV file (see transcribe.go).
func writeWAVHeader(w io.Writer, dataSize int64, sampleRate uint32, channels, bitsPerSample uint16) error {
	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample) / 8
	blockAlign := channels * bitsPerSample / 8

	buf := make([]byte, 44)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataSize))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(buf[22:24], channels)
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], byteRate)
	binary.LittleEndian.PutUint16(buf[32:34], blockAlign)
	binary.LittleEndian.PutUint16(buf[34:36], bitsPerSample)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))

	_, err := w.Write(buf)
	return err
}
