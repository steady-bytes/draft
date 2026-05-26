package broker

import (
	"context"
	"encoding/base64"
	"sync"

	"connectrpc.com/connect"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

type (
	atomicMap struct {
		mu sync.RWMutex
		// Store the routine client connection
		m map[string][]*connect.ServerStream[acv1.ConsumeResponse]
		// yield the client connection to a thread, and then send events to it
		n map[string]chan *acv1.CloudEvent
		// registrations maps event_type → []consumer_source_names for topology tracking
		registrations map[string][]string
	}
)

func newAtomicMap() *atomicMap {
	return &atomicMap{
		mu:            sync.RWMutex{},
		m:             make(map[string][]*connect.ServerStream[acv1.ConsumeResponse]),
		n:             make(map[string]chan *acv1.CloudEvent),
		registrations: make(map[string][]string),
	}
}

// hash to calculate the same key for two strings
func (am *atomicMap) hash(msgKindName string) string {
	bs := []byte(msgKindName)
	return base64.StdEncoding.EncodeToString(bs)
}

func (am *atomicMap) Insert(key string, resStream *connect.ServerStream[acv1.ConsumeResponse]) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.m[key] = append(am.m[key], resStream)
}

func (am *atomicMap) Broker(ctx context.Context, key string, resStream *connect.ServerStream[acv1.ConsumeResponse]) {
	am.mu.RLock()
	ch, found := am.n[key]
	if !found {
		// create the channel to add to map
		ch := make(chan *acv1.CloudEvent)
		// store channel in map for future connections
		am.mu.RUnlock()
		am.mu.Lock()
		am.n[key] = ch
		am.mu.Unlock()
		// now start a new routine and keep it open as long as the `ch` channel has connected clients
		go am.send(ctx, ch, resStream)

		return
	} else {
		// the channel is already made and shared with other consumers, and producers so we can just use `ch`
		go am.send(ctx, ch, resStream)
		am.mu.RUnlock()
	}
}

func (am *atomicMap) send(ctx context.Context, ch chan *acv1.CloudEvent, stream *connect.ServerStream[acv1.ConsumeResponse]) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-ch:
			if err := stream.Send(&acv1.ConsumeResponse{Message: m}); err != nil {
				return
			}
		}
	}
}

func (am *atomicMap) Broadcast(key string, msg *acv1.CloudEvent) {
	am.mu.RLock()
	ch, ok := am.n[key]
	am.mu.RUnlock()
	if ok {
		ch <- msg
	}
	// No consumers registered for this key — drop silently.
	// TODO: consider a dead-letter queue.
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
