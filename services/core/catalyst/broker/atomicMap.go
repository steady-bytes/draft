package broker

import (
	"sync"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

// subChanCapacity buffers a consumer's own delivery channel so a brief pause (a slow network
// write, a GC pause) doesn't drop a message that would have been delivered a moment later —
// matching observerRegistry's own QueryStream subscriber channels (controller.go) at the same
// size.
const subChanCapacity = 64

type (
	// consumerSub is one open Consume stream's own delivery channel, and the event type it
	// declared when it registered. An empty eventType is a wildcard — every event reaches it,
	// matching a debug/catch-all consumer such as `dctl broker consume` run with no --type flag.
	consumerSub struct {
		eventType string
		ch        chan *acv1.CloudEvent
	}

	// atomicMap holds the live Consume subscriptions Broadcast delivers into, plus the
	// (event_type → consumer source names) bookkeeping GetTopology reads. The two are unrelated:
	// the subscriptions below are what a message is actually delivered to; registrations is purely
	// descriptive, keyed by each consumer's own declared identity.
	atomicMap struct {
		mu      sync.RWMutex
		subs    map[uint64]*consumerSub
		counter uint64

		registrations map[string][]string
	}
)

func newAtomicMap() *atomicMap {
	return &atomicMap{
		subs:          make(map[uint64]*consumerSub),
		registrations: make(map[string][]string),
	}
}

// Subscribe registers a new consumer for eventType (empty = every type) and returns its own
// delivery channel, plus the id to Unsubscribe it with once the consumer disconnects.
func (am *atomicMap) Subscribe(eventType string) (uint64, chan *acv1.CloudEvent) {
	ch := make(chan *acv1.CloudEvent, subChanCapacity)
	am.mu.Lock()
	id := am.counter
	am.counter++
	am.subs[id] = &consumerSub{eventType: eventType, ch: ch}
	am.mu.Unlock()
	return id, ch
}

// Unsubscribe removes one consumer's delivery channel. Idempotent — safe to call once a consumer
// has already been removed (or was never added).
func (am *atomicMap) Unsubscribe(id uint64) {
	am.mu.Lock()
	delete(am.subs, id)
	am.mu.Unlock()
}

// Broadcast delivers msg to every consumer subscribed to msg.GetType(), plus every wildcard
// consumer (declared with an empty type) — see this method's own history for why "every", not
// "one": until 2026-09-27 this delivered to exactly one arbitrarily-chosen subscriber per message,
// not every interested one (docs/website/content/docs/architecture/core-services.md's "Known
// issues" documented it; see that section's git history for the original writeup). The two bugs
// combining to cause it are both gone now: routing used to key on the CloudEvent envelope's own Go
// type name (always the same value, "CloudEvent", for every message regardless of its actual
// `.Type`), collapsing every producer and consumer in the cluster onto one shared bucket; and that
// bucket delivered through one shared, unbuffered channel read by every open Consume stream's own
// forwarding goroutine, so a single `ch <- msg` was received by whichever goroutine happened to be
// ready first — not fanned out to the rest. Each subscriber now has its own buffered channel (see
// Subscribe), so a message reaches every one of them independently. A slow or stalled consumer is
// skipped rather than blocking the producer loop or the other consumers, the same non-blocking-drop
// discipline observerRegistry.broadcast already uses for QueryStream subscribers, just above.
func (am *atomicMap) Broadcast(msg *acv1.CloudEvent) {
	typ := msg.GetType()
	am.mu.RLock()
	defer am.mu.RUnlock()
	for _, sub := range am.subs {
		if sub.eventType != "" && sub.eventType != typ {
			continue
		}
		select {
		case sub.ch <- msg:
		default:
		}
	}
}

// AddRegistration records that source is subscribed to eventType.
// Duplicate entries for the same (eventType, source) pair are ignored.
func (am *atomicMap) AddRegistration(eventType, source string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	for _, s := range am.registrations[eventType] {
		if s == source {
			return
		}
	}
	am.registrations[eventType] = append(am.registrations[eventType], source)
}

// RemoveRegistration removes source from the subscriber list for eventType.
func (am *atomicMap) RemoveRegistration(eventType, source string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	list := am.registrations[eventType]
	for i, s := range list {
		if s == source {
			updated := append(list[:i], list[i+1:]...)
			if len(updated) == 0 {
				delete(am.registrations, eventType)
			} else {
				am.registrations[eventType] = updated
			}
			return
		}
	}
}

// ConsumerCount returns the number of distinct consumer sources currently registered.
func (am *atomicMap) ConsumerCount() int {
	am.mu.RLock()
	defer am.mu.RUnlock()
	seen := make(map[string]bool)
	for _, srcs := range am.registrations {
		for _, s := range srcs {
			seen[s] = true
		}
	}
	return len(seen)
}

// Registrations returns a snapshot copy of the full event_type → consumers map.
func (am *atomicMap) Registrations() map[string][]string {
	am.mu.RLock()
	defer am.mu.RUnlock()
	out := make(map[string][]string, len(am.registrations))
	for k, v := range am.registrations {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
