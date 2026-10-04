package service

import (
	"sync"

	relayv1 "github.com/steady-bytes/draft/api/tooling/relay/v1"
)

// BrowserOutputDeviceID is Phase 3's one and only output device -- "Stream to browser" in
// mockups/pages/relay-library.html. Every Play/Pause/Seek/SetVolume/GetPlayerState call operates
// on the single shared Player below regardless of what output_device_id a request names; real
// per-device routing arrives with native hardware support in Phase 9 (the plan's own "one shared
// PlayerState per output device" decision -- Phase 3 just has exactly one such device to share).
//
// Distinct from BrowserInputDeviceID (session.go) despite the similar name and both describing
// "the browser" -- they used to share the literal string "browser" until Phase 9's ListDevices
// put both in the same list and a duplicate-id bug became directly visible (two Device entries,
// one input one output, both claiming id "browser"). Kept as two constants, not one, since an
// input device and an output device are conceptually different things even when both happen to be
// "the browser" -- a caller should never need to guess which one a bare "browser" id meant.
const BrowserOutputDeviceID = "browser-out"

// Player is Relay's in-process playback state machine for the browser output device. It holds no
// audio itself -- rpc.go's StreamAudioOut reads bytes from a Track's file and paces them out to
// the browser, consulting Player's state as it goes.
//
// generation increments on every call that can invalidate an in-flight send loop (a new Play,
// Pause, Seek): StreamAudioOut remembers the generation in effect when it started sending a given
// track, and stops as soon as Advance reports it's no longer current, so it never keeps pushing
// audio that a concurrent control call has already superseded. This is the same
// coalesce-to-latest-state shape as services/core/blueprint/service_discovery/broadcaster.go's
// fix (see the "Blueprint Service Registry staleness" memory), applied here to guard a
// long-running send loop instead of a Watch subscriber's buffered channel.
type Player struct {
	mu         sync.Mutex
	trackID    string
	positionMs int64
	playing    bool
	volumePct  int32
	generation uint64
	wake       chan struct{} // 1-slot: StreamAudioOut blocks here while idle/paused
}

func NewPlayer() *Player {
	return &Player{volumePct: 100, wake: make(chan struct{}, 1)}
}

// Wake is the channel StreamAudioOut blocks on while nothing is playing. It's woken (non-blocking)
// by every state change that could start playback.
func (p *Player) Wake() <-chan struct{} {
	return p.wake
}

func (p *Player) notify() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Snapshot is the state StreamAudioOut reads at the top of each pass through its send loop.
type Snapshot struct {
	TrackID    string
	PositionMs int64
	Playing    bool
	Generation uint64
}

func (p *Player) Snapshot() Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Snapshot{TrackID: p.trackID, PositionMs: p.positionMs, Playing: p.playing, Generation: p.generation}
}

func (p *Player) State() *relayv1.PlayerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return &relayv1.PlayerState{
		TrackId:        p.trackID,
		PositionMs:     p.positionMs,
		Playing:        p.playing,
		VolumePct:      p.volumePct,
		OutputDeviceId: BrowserOutputDeviceID,
	}
}

func (p *Player) Play(trackID string) *relayv1.PlayerState {
	p.mu.Lock()
	if trackID != "" && trackID != p.trackID {
		p.trackID = trackID
		p.positionMs = 0
	}
	p.playing = true
	p.generation++
	p.mu.Unlock()
	p.notify()
	return p.State()
}

func (p *Player) Pause() *relayv1.PlayerState {
	p.mu.Lock()
	p.playing = false
	p.generation++
	p.mu.Unlock()
	p.notify()
	return p.State()
}

func (p *Player) Seek(positionMs int64) *relayv1.PlayerState {
	p.mu.Lock()
	p.positionMs = positionMs
	p.generation++
	p.mu.Unlock()
	p.notify()
	return p.State()
}

func (p *Player) SetVolume(volumePct int32) *relayv1.PlayerState {
	p.mu.Lock()
	p.volumePct = volumePct
	p.mu.Unlock()
	// No generation bump/notify: volume is a client-side gain applied to bytes the browser
	// already has, not something that invalidates an in-flight send loop.
	return p.State()
}

// Advance moves positionMs forward by deltaMs as StreamAudioOut sends bytes, but only if
// generation still matches expectedGen. A mismatch means some other call (Pause/Seek/a new Play)
// raced with this send and should win; the caller is expected to stop sending and re-snapshot.
func (p *Player) Advance(expectedGen uint64, deltaMs int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.generation != expectedGen {
		return false
	}
	p.positionMs += deltaMs
	return true
}

// Finished marks playback stopped because the track ran out on its own, but only if generation
// still matches expectedGen -- an intervening Seek/Pause/Play already changed what "stopped"
// should mean, and that call's own state wins instead.
func (p *Player) Finished(expectedGen uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.generation != expectedGen {
		return
	}
	p.playing = false
	p.positionMs = 0
	p.generation++
}
