// watch.go: in-process pub/sub feeding the Watch RPC. Deliberately not a
// Catalyst subscription -- see docs/architecture/lineman-implementation-plan.md's
// Decisions on Catalyst's Consume fan-out bug. Every mutating call in this
// service publishes here after a successful write; every open Watch stream
// gets its own buffered channel and receives every publish, so delivery
// isn't subject to the "one random subscriber" behavior Catalyst has today.
package main

import (
	"sync"

	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"
)

// watchBufferSize is generous enough that a slow client doesn't miss events
// under normal load; a client that falls behind by this many events is
// disconnected rather than blocking every other publisher (see publish).
const watchBufferSize = 64

type watchBroadcaster struct {
	mu   sync.Mutex
	subs map[chan *linemanv1.WatchResponse]struct{}
}

func newWatchBroadcaster() *watchBroadcaster {
	return &watchBroadcaster{subs: make(map[chan *linemanv1.WatchResponse]struct{})}
}

// subscribe registers a new channel and returns it plus an unsubscribe func.
// Call unsubscribe when the Watch stream's context is done.
func (b *watchBroadcaster) subscribe() (<-chan *linemanv1.WatchResponse, func()) {
	ch := make(chan *linemanv1.WatchResponse, watchBufferSize)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
		close(ch)
	}
}

// publish fans item out to every currently-subscribed Watch stream. A
// subscriber whose buffer is already full is skipped for this event (never
// blocks the publisher, and never drops every subscriber for one slow one).
func (b *watchBroadcaster) publish(resp *linemanv1.WatchResponse) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- resp:
		default:
		}
	}
}

func (b *watchBroadcaster) publishTask(t *linemanv1.Task, removed bool) {
	b.publish(&linemanv1.WatchResponse{Item: &linemanv1.WatchResponse_Task{Task: t}, Removed: removed})
}

func (b *watchBroadcaster) publishAgent(a *linemanv1.Agent) {
	b.publish(&linemanv1.WatchResponse{Item: &linemanv1.WatchResponse_Agent{Agent: a}})
}

func (b *watchBroadcaster) publishScheduledTask(s *linemanv1.ScheduledTask) {
	b.publish(&linemanv1.WatchResponse{Item: &linemanv1.WatchResponse_ScheduledTask{ScheduledTask: s}})
}

func (b *watchBroadcaster) publishLoop(l *linemanv1.Loop) {
	b.publish(&linemanv1.WatchResponse{Item: &linemanv1.WatchResponse_Loop{Loop: l}})
}
