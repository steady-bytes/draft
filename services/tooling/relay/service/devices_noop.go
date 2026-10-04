//go:build !portaudio

package service

import relayv1 "github.com/steady-bytes/draft/api/tooling/relay/v1"

// NewDeviceLister returns a DeviceLister that reports no native hardware. This is the default
// build (no "portaudio" build tag) -- zero cgo dependencies, exactly how this service already ran
// through Phases 1-8. Build with `-tags portaudio` (PortAudio installed) for real enumeration; see
// devices_portaudio.go.
func NewDeviceLister() (DeviceLister, error) {
	return &noopDeviceLister{}, nil
}

type noopDeviceLister struct{}

func (l *noopDeviceLister) ListDevices() ([]*relayv1.Device, error) {
	return nil, nil
}

func (l *noopDeviceLister) Close() error {
	return nil
}
