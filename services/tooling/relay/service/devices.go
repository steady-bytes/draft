package service

import relayv1 "github.com/steady-bytes/draft/api/tooling/relay/v1"

// DeviceLister enumerates this instance's own local hardware audio devices -- see the
// implementation plan's Multi-instance device ownership section: each Relay instance answers
// ListDevices with ONLY its own devices, never a cluster-wide registry (aggregating across
// instances is a caller-side concern -- Query Blueprint for every "relay" instance, call
// ListDevices on each -- not something this service does to itself).
//
// Real hardware enumeration (devices_portaudio.go) needs PortAudio, a cgo dependency, and is
// opt-in via the "portaudio" build tag: `go build -tags portaudio .` (with PortAudio installed,
// e.g. `brew install portaudio` on macOS). The default build (devices_noop.go, no build tag)
// reports no native hardware, keeping this service's zero-cgo dependency footprint through
// Phases 1-8 unchanged for every deployment that only cares about browser-streamed audio.
type DeviceLister interface {
	// ListDevices returns this instance's own real native hardware devices only -- NOT the
	// virtual browser input/output devices (BrowserInputDeviceID/BrowserOutputDeviceID), which
	// rpc.go's ListDevices handler always adds on top, regardless of what this returns.
	ListDevices() ([]*relayv1.Device, error)
	// Close releases whatever process-wide audio resources NewDeviceLister acquired (PortAudio's
	// Initialize/Terminate pairing) -- registered as a chassis.Effect dispose in main.go, not
	// called directly by anything else.
	Close() error
}
