// Command list-devices is a small standalone check for Relay's Phase 9 hardware enumeration
// (service/devices*.go) -- prints this machine's real native audio devices without spinning up
// the full service (no Blueprint registration, no Postgres connection, no Foundry publish), useful
// for verifying a PortAudio install works before ever starting Relay itself.
//
// Default build (no tags): reports no native hardware, matching Relay's own default.
// `go run -tags portaudio ./cmd/list-devices` (PortAudio installed): enumerates real hardware.
package main

import (
	"fmt"
	"os"

	"github.com/steady-bytes/draft/services/tooling/relay/service"
)

func main() {
	lister, err := service.NewDeviceLister()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to initialize device lister:", err)
		os.Exit(1)
	}
	defer lister.Close()

	devices, err := lister.ListDevices()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to list devices:", err)
		os.Exit(1)
	}

	if len(devices) == 0 {
		fmt.Println("no native devices found (built without -tags portaudio, or none attached)")
		return
	}
	for _, d := range devices {
		fmt.Printf("%-14s %-40s %-6s channels=%d sample_rate_hz=%d\n", d.GetId(), d.GetName(), d.GetKind(), d.GetChannels(), d.GetSampleRateHz())
	}
}
