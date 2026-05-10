package broker

import (
	"context"
	"sync"

	"connectrpc.com/connect"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	Controller interface {
		Consumer
		Producer
		Query(ctx context.Context, req *acv1.QueryRequest) ([]*acv1.CloudEvent, error)
		QueryStream(ctx context.Context, req *acv1.QueryRequest, stream *connect.ServerStream[acv1.QueryStreamResponse]) error
	}

	controller struct {
		Producer
		Consumer

		logger    chassis.Logger
		state     *atomicMap
		storer    Storer
		observers *observerRegistry
	}

	register struct {
		*acv1.CloudEvent
		*connect.ServerStream[acv1.ConsumeResponse]
	}

	// observerRegistry manages live-event channels for QueryStream subscribers.
	observerRegistry struct {
		mu      sync.RWMutex
		subs    map[uint64]chan *acv1.CloudEvent
		counter uint64
	}
)

func NewController(logger chassis.Logger, storer Storer) Controller {
	var (
		producerMsgChan          = make(chan *acv1.CloudEvent)
		consumerRegistrationChan = make(chan register)
	)

	ctr := &controller{
		Producer:  NewProducer(producerMsgChan),
		Consumer:  NewConsumer(consumerRegistrationChan),
		logger:    logger,
		state:     newAtomicMap(),
		storer:    storer,
		observers: newObserverRegistry(),
	}

	go ctr.produce(producerMsgChan)
	go ctr.consume(consumerRegistrationChan)

	return ctr
}

func newObserverRegistry() *observerRegistry {
	return &observerRegistry{subs: make(map[uint64]chan *acv1.CloudEvent)}
}

func (r *observerRegistry) subscribe() (uint64, chan *acv1.CloudEvent) {
	ch := make(chan *acv1.CloudEvent, 64)
	r.mu.Lock()
	id := r.counter
	r.counter++
	r.subs[id] = ch
	r.mu.Unlock()
	return id, ch
}

func (r *observerRegistry) unsubscribe(id uint64) {
	r.mu.Lock()
	delete(r.subs, id)
	r.mu.Unlock()
}

// broadcast sends event to every active QueryStream subscriber. Slow consumers
// are skipped rather than blocking the producer goroutine.
func (r *observerRegistry) broadcast(event *acv1.CloudEvent) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ch := range r.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

const (
	LOG_KEY_TO_CH = "key to channel"
)

func (c *controller) produce(producerMsgChan chan *acv1.CloudEvent) {
	for {
		msg := <-producerMsgChan
		c.logger.WithField("msg", msg).Info("produce message received")

		if err := c.storer.Save(context.Background(), msg); err != nil {
			c.logger.WithField("error", err.Error()).Error("failed to persist event")
		}

		key := c.state.hash(string(msg.ProtoReflect().Descriptor().FullName()))
		c.logger.WithField("key", key).Info(LOG_KEY_TO_CH)
		c.state.Broadcast(key, msg)
		c.observers.broadcast(msg)
	}
}

func (c *controller) consume(registerChan chan register) {
	for {
		msg := <-registerChan
		c.logger.WithField("msg", msg).Info("consume channel registration")

		key := c.state.hash(string(msg.ProtoReflect().Descriptor().FullName()))
		c.logger.WithField("key", key).Info(LOG_KEY_TO_CH)
		c.state.Broker(key, msg.ServerStream)
	}
}

func (c *controller) Query(ctx context.Context, req *acv1.QueryRequest) ([]*acv1.CloudEvent, error) {
	candidates, err := c.storer.Query(ctx, req.GetLimit(), req.GetAfter())
	if err != nil {
		return nil, err
	}

	expr := req.GetExpression()
	if expr == nil {
		return candidates, nil
	}

	matched := make([]*acv1.CloudEvent, 0, len(candidates))
	for _, event := range candidates {
		if ok, _ := matchesExpression(expr, event); ok {
			matched = append(matched, event)
		}
	}
	return matched, nil
}

func (c *controller) QueryStream(ctx context.Context, req *acv1.QueryRequest, stream *connect.ServerStream[acv1.QueryStreamResponse]) error {
	expr := req.GetExpression()

	// Phase 1: replay historical events from ClickHouse.
	historical, err := c.storer.Query(ctx, req.GetLimit(), req.GetAfter())
	if err != nil {
		return err
	}
	for _, event := range historical {
		if ok, _ := matchesExpression(expr, event); ok {
			if err := stream.Send(&acv1.QueryStreamResponse{Event: event}); err != nil {
				return err
			}
		}
	}

	// Phase 2: stream live events as they arrive.
	id, ch := c.observers.subscribe()
	defer c.observers.unsubscribe(id)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-ch:
			if ok, _ := matchesExpression(expr, event); ok {
				if err := stream.Send(&acv1.QueryStreamResponse{Event: event}); err != nil {
					return err
				}
			}
		}
	}
}
