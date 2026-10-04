//go:build portaudio

package service

import (
	"fmt"

	relayv1 "github.com/steady-bytes/draft/api/tooling/relay/v1"

	"github.com/gordonklaus/portaudio"
)

// portaudioDeviceLister enumerates this machine's real, physically-attached audio hardware via
// PortAudio (cgo bindings, the plan's own recommendation -- see Multi-instance device ownership).
// A physical device that supports both directions (rare, but possible) gets two separate Device
// entries, one per direction, matching the proto's own one-kind-per-Device shape -- there is no
// combined-input-and-output Device message.
type portaudioDeviceLister struct{}

func NewDeviceLister() (DeviceLister, error) {
	if err := portaudio.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize PortAudio: %w", err)
	}
	return &portaudioDeviceLister{}, nil
}

func (l *portaudioDeviceLister) ListDevices() ([]*relayv1.Device, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate PortAudio devices: %w", err)
	}

	result := make([]*relayv1.Device, 0, len(devices))
	for i, d := range devices {
		if d.MaxInputChannels > 0 {
			result = append(result, &relayv1.Device{
				Id:           fmt.Sprintf("native-%d-in", i),
				Name:         d.Name,
				Kind:         relayv1.DeviceKind_DEVICE_KIND_INPUT,
				Channels:     int32(d.MaxInputChannels),
				SampleRateHz: int32(d.DefaultSampleRate),
			})
		}
		if d.MaxOutputChannels > 0 {
			result = append(result, &relayv1.Device{
				Id:           fmt.Sprintf("native-%d-out", i),
				Name:         d.Name,
				Kind:         relayv1.DeviceKind_DEVICE_KIND_OUTPUT,
				Channels:     int32(d.MaxOutputChannels),
				SampleRateHz: int32(d.DefaultSampleRate),
			})
		}
	}
	return result, nil
}

func (l *portaudioDeviceLister) Close() error {
	return portaudio.Terminate()
}
