package service_discovery

import (
	"sync"

	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"

	"github.com/google/uuid"
)

type ProcessEvent struct {
	Process *sdv1.Process
	Removed bool
}

type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[string]chan *ProcessEvent
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[string]chan *ProcessEvent),
	}
}

func (b *Broadcaster) Subscribe() (string, <-chan *ProcessEvent) {
	id := uuid.NewString()
	ch := make(chan *ProcessEvent, 16)
	b.mu.Lock()
	b.subscribers[id] = ch
	b.mu.Unlock()
	return id, ch
}

func (b *Broadcaster) Unsubscribe(id string) {
	b.mu.Lock()
	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
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
	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}
