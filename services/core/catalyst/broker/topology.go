package broker

import (
	"sync"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

// topologyObserver holds a buffered channel for one WatchTopology subscriber.
type topologyObserver struct {
	ch chan *acv1.WatchTopologyResponse
}

// topologyRegistry tracks WatchTopology subscribers and the set of producer
// sources that have been observed at least once. It is safe for concurrent use.
type topologyRegistry struct {
	mu        sync.RWMutex
	observers []*topologyObserver
	seen      map[string]bool // producer sources seen in the produce loop
}

func newTopologyRegistry() *topologyRegistry {
	return &topologyRegistry{
		observers: make([]*topologyObserver, 0),
		seen:      make(map[string]bool),
	}
}

// subscribe registers a new WatchTopology stream and returns the observer plus
// a cleanup function that must be called (typically via defer) to deregister it.
func (r *topologyRegistry) subscribe() (*topologyObserver, func()) {
	obs := &topologyObserver{ch: make(chan *acv1.WatchTopologyResponse, 64)}
	r.mu.Lock()
	r.observers = append(r.observers, obs)
	r.mu.Unlock()
	return obs, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for i, o := range r.observers {
			if o == obs {
				r.observers = append(r.observers[:i], r.observers[i+1:]...)
				return
			}
		}
	}
}

// broadcast sends resp to every active subscriber. Slow consumers are skipped
// rather than blocking the caller.
func (r *topologyRegistry) broadcast(resp *acv1.WatchTopologyResponse) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, obs := range r.observers {
		select {
		case obs.ch <- resp:
		default:
		}
	}
}

// NotifyConsumerConnected broadcasts a consumer_connected delta.
func (r *topologyRegistry) NotifyConsumerConnected(source string) {
	r.broadcast(&acv1.WatchTopologyResponse{
		Event: &acv1.WatchTopologyResponse_ConsumerConnected{
			ConsumerConnected: &acv1.TopologyNode{Id: source, Name: source},
		},
	})
}

// NotifyConsumerDisconnected broadcasts a consumer_disconnected delta.
func (r *topologyRegistry) NotifyConsumerDisconnected(source string) {
	r.broadcast(&acv1.WatchTopologyResponse{
		Event: &acv1.WatchTopologyResponse_ConsumerDisconnected{
			ConsumerDisconnected: &acv1.TopologyNode{Id: source, Name: source},
		},
	})
}

// ProducerCount returns the number of distinct producer sources seen since start.
func (r *topologyRegistry) ProducerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.seen)
}

// ObserveProducer broadcasts producer_appeared the first time source is seen.
func (r *topologyRegistry) ObserveProducer(source string) {
	r.mu.Lock()
	if r.seen[source] {
		r.mu.Unlock()
		return
	}
	r.seen[source] = true
	r.mu.Unlock()
	r.broadcast(&acv1.WatchTopologyResponse{
		Event: &acv1.WatchTopologyResponse_ProducerAppeared{
			ProducerAppeared: &acv1.TopologyNode{Id: source, Name: source},
		},
	})
}
