package service_discovery

import (
	"sync"

	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"

	"github.com/google/uuid"
)

// ProcessEvent is one change Broadcaster delivers to a Watch subscriber: either process was
// updated (a fresh heartbeat, a health-state change, a new registration, ...) or Process.Pid was
// removed (Finalize, or the reaper deregistering it after DeregisterThreshold).
type ProcessEvent struct {
	Process *sdv1.Process
	Removed bool
}

// subscriber is one open Watch call's delivery state.
//
// Publishes are coalesced by pid, not queued as a raw event log: only the *latest* event for a
// given process is ever pending at once. This replaces an earlier design (a fixed 16-slot
// buffered channel per subscriber, fed by a non-blocking send that silently dropped the event on
// a full buffer) that had a real, confirmed-live bug: ~20 processes each heartbeating every
// chassis.SYNC_INTERVAL (5s) is enough aggregate volume that a subscriber briefly slower than the
// browser's own event loop would miss updates outright, with no way to ever recover them (Query
// only runs once, at page load) -- verified live by querying Blueprint directly while its own
// Service Registry page showed 4 of its 5 raft nodes as "Stale" with correct, current
// last_status_time server-side. See docs/website/content/docs/architecture/... (service registry
// section) if this gets written up there.
//
// Coalescing by pid is the right fix, not just a bigger buffer, because a Watch subscriber never
// actually needs every intermediate heartbeat -- only the most recent state of each process. A
// pending map bounded by "how many distinct processes exist" (tens, not thousands) can never
// overflow the way a fixed-size queue of raw events can under a burst, no matter how far behind a
// slow subscriber temporarily falls.
type subscriber struct {
	mu      sync.Mutex
	pending map[string]*ProcessEvent // keyed by Process.Pid
	// wake signals "pending has new data" -- capacity 1 is enough since it's a level trigger, not
	// a data channel: rpc.go's Watch loop drains the *whole* pending map on every wake, so several
	// wakes queued up before it gets around to draining are redundant, never lost information (the
	// data itself lives in `pending`, already coalesced).
	wake chan struct{}
}

func newSubscriber() *subscriber {
	return &subscriber{
		pending: make(map[string]*ProcessEvent),
		wake:    make(chan struct{}, 1),
	}
}

// publish coalesces event into this subscriber's pending set (overwriting any not-yet-delivered
// event for the same pid) and wakes its Watch loop if it isn't already awake.
func (s *subscriber) publish(event *ProcessEvent) {
	s.mu.Lock()
	s.pending[event.Process.GetPid()] = event
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// drain removes and returns every currently-pending event. Order across different pids is not
// meaningful (map iteration order) -- a Watch subscriber only cares about each pid's latest state,
// never about the relative delivery order of two different processes' updates.
func (s *subscriber) drain() []*ProcessEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return nil
	}
	out := make([]*ProcessEvent, 0, len(s.pending))
	for _, e := range s.pending {
		out = append(out, e)
	}
	s.pending = make(map[string]*ProcessEvent)
	return out
}

// Broadcaster fans out process registry changes to every open Watch call. See subscriber's doc
// comment for why delivery is coalesce-and-wake rather than a buffered channel of raw events.
type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriber
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[string]*subscriber),
	}
}

// Subscribe registers a new Watch call and returns its id (pass to Drain and Unsubscribe) and the
// channel to select on: a receive from it means Drain(id) has something to return.
func (b *Broadcaster) Subscribe() (string, <-chan struct{}) {
	id := uuid.NewString()
	sub := newSubscriber()
	b.mu.Lock()
	b.subscribers[id] = sub
	b.mu.Unlock()
	return id, sub.wake
}

// Drain returns everything pending for subscriber id since the last Drain call, or nil if id is
// unknown (e.g. raced with its own Unsubscribe) or has nothing pending.
func (b *Broadcaster) Drain(id string) []*ProcessEvent {
	b.mu.RLock()
	sub, ok := b.subscribers[id]
	b.mu.RUnlock()
	if !ok {
		return nil
	}
	return sub.drain()
}

// Unsubscribe removes subscriber id. Idempotent. A publish already in flight for this id (holding
// a reference to the subscriber obtained before this call) may still write into its now-detached
// pending map harmlessly -- nothing reads it again, and it's garbage the next time the caller's
// own local `sub` variable goes out of scope.
func (b *Broadcaster) Unsubscribe(id string) {
	b.mu.Lock()
	delete(b.subscribers, id)
	b.mu.Unlock()
}

func (b *Broadcaster) Publish(process *sdv1.Process) {
	b.publish(&ProcessEvent{Process: process, Removed: false})
}

func (b *Broadcaster) PublishRemoved(pid string) {
	b.publish(&ProcessEvent{Process: &sdv1.Process{Pid: pid}, Removed: true})
}

func (b *Broadcaster) publish(event *ProcessEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subscribers {
		sub.publish(event)
	}
}
