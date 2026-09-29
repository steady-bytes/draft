package broker

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	Controller interface {
		Consumer
		Producer
		Query(ctx context.Context, req *acv1.QueryRequest) ([]*acv1.CloudEvent, error)
		QueryStream(ctx context.Context, req *acv1.QueryRequest, stream *connect.ServerStream[acv1.QueryStreamResponse]) error
		GetTopology(ctx context.Context) (*acv1.GetTopologyResponse, error)
		WatchTopology(ctx context.Context, stream *connect.ServerStream[acv1.WatchTopologyResponse]) error
		GetMetrics(ctx context.Context, windowSecs int32) (*acv1.GetMetricsResponse, error)
		GetResourceMetrics(ctx context.Context) (*acv1.GetResourceMetricsResponse, error)
		GetTopicSeries(ctx context.Context, eventType string, windowSecs int32) (*acv1.GetTopicSeriesResponse, error)
	}

	controller struct {
		Producer
		Consumer

		logger          chassis.Logger
		state           *atomicMap
		storer          Storer
		observers       *observerRegistry
		topology        *topologyRegistry
		publishedTotal  atomic.Uint64
	}

	register struct {
		ctx context.Context
		*acv1.CloudEvent
		*connect.ServerStream[acv1.ConsumeResponse]
		// subscribed carries back the subscription consumer.Consume must deliver into, once
		// controller.consume has called Subscribe for it. Buffered by 1 so consume() never blocks
		// handing it off.
		subscribed chan subscription
	}

	// subscription is one Consume call's own delivery channel and the id to Unsubscribe it with —
	// see atomicMap.Subscribe.
	subscription struct {
		id uint64
		ch chan *acv1.CloudEvent
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

	state := newAtomicMap()
	ctr := &controller{
		Producer:  NewProducer(producerMsgChan),
		Consumer:  NewConsumer(consumerRegistrationChan, state),
		logger:    logger,
		state:     state,
		storer:    storer,
		observers: newObserverRegistry(),
		topology:  newTopologyRegistry(),
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


func (c *controller) produce(producerMsgChan chan *acv1.CloudEvent) {
	for {
		msg := <-producerMsgChan
		receivedAt := time.Now()

		// Stamp forwarded_at before broadcasting so live QueryStream subscribers
		// receive the attribute alongside the event payload.
		forwardedAt := time.Now()
		if msg.Attributes == nil {
			msg.Attributes = make(map[string]*acv1.CloudEvent_CloudEventAttributeValue)
		}
		msg.Attributes["forwarded_at"] = &acv1.CloudEvent_CloudEventAttributeValue{
			Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{
				CeTimestamp: timestamppb.New(forwardedAt),
			},
		}

		c.publishedTotal.Add(1)
		c.state.Broadcast(msg)
		c.observers.broadcast(msg)
		c.topology.ObserveProducer(msg.GetSource())

		if err := c.storer.Save(context.Background(), msg, receivedAt, forwardedAt); err != nil {
			c.logger.WithField("error", err.Error()).Error("failed to persist event")
		}
	}
}

// consume hands each registration its subscription (see atomicMap.Subscribe) and tracks consumer
// identity for topology — it never touches reg.ServerStream itself. Actual message delivery
// (stream.Send) happens back on consumer.Consume's own goroutine, the same one the connect
// framework will later call Close on for this RPC: an earlier version of this fix ran delivery in
// a second goroutine here, which raced with that framework-driven Close over the same underlying
// HTTP/2 response writer (a -race failure, not a correctness one, but a real one) whenever the
// consumer's context was cancelled while a message was in flight.
func (c *controller) consume(registerChan chan register) {
	for {
		reg := <-registerChan

		// reg.GetType() is the event type this consumer declared interest in (empty = every
		// type — see atomicMap.Subscribe).
		id, ch := c.state.Subscribe(reg.GetType())
		reg.subscribed <- subscription{id: id, ch: ch}

		// Track consumer identity declared via ConsumeRequest.message.source/type.
		// A cleanup goroutine removes the registration when the RPC context is cancelled
		// (i.e. the client disconnects).
		src := reg.GetSource()
		typ := reg.GetType()
		if src != "" && typ != "" {
			c.state.AddRegistration(typ, src)
			c.topology.NotifyConsumerConnected(src)
			go func(ctx context.Context, src, typ string) {
				<-ctx.Done()
				c.state.RemoveRegistration(typ, src)
				c.topology.NotifyConsumerDisconnected(src)
			}(reg.ctx, src, typ)
		}
	}
}

func (c *controller) Query(ctx context.Context, req *acv1.QueryRequest) ([]*acv1.CloudEvent, error) {
	descending := req.GetOrderBy() == acv1.OrderDirection_ORDER_DIRECTION_DESC
	candidates, err := c.storer.Query(ctx, req.GetLimit(), req.GetAfter(), descending)
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
	descending := req.GetOrderBy() == acv1.OrderDirection_ORDER_DIRECTION_DESC
	historical, err := c.storer.Query(ctx, req.GetLimit(), req.GetAfter(), descending)
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

func (c *controller) GetTopology(ctx context.Context) (*acv1.GetTopologyResponse, error) {
	producers, sourcesByType, err := c.storer.QueryDistinctSources(ctx)
	if err != nil {
		return nil, err
	}

	producerNodes := make([]*acv1.TopologyNode, len(producers))
	for i, src := range producers {
		producerNodes[i] = &acv1.TopologyNode{Id: src, Name: src}
	}

	regs := c.state.Registrations()
	consumerSet := make(map[string]bool)
	var edges []*acv1.TopologyEdge

	for eventType, consumers := range regs {
		producerSrcs := sourcesByType[eventType]
		for _, consumerSrc := range consumers {
			consumerSet[consumerSrc] = true
			for _, producerSrc := range producerSrcs {
				edges = append(edges, &acv1.TopologyEdge{
					ProducerSource: producerSrc,
					ConsumerSource: consumerSrc,
					EventType:      eventType,
				})
			}
		}
	}

	// Enrich edges with per-minute vol counts from ClickHouse (60-second window).
	vols, _ := c.storer.QueryVolumes(ctx, 60)
	volIdx := make(map[string]map[string]uint32)
	for _, v := range vols {
		if volIdx[v.Source] == nil {
			volIdx[v.Source] = make(map[string]uint32)
		}
		volIdx[v.Source][v.EventType] += v.Count
	}
	for _, e := range edges {
		if byType, ok := volIdx[e.ProducerSource]; ok {
			e.Vol = byType[e.EventType]
		}
	}

	consumerNodes := make([]*acv1.TopologyNode, 0, len(consumerSet))
	for src := range consumerSet {
		consumerNodes = append(consumerNodes, &acv1.TopologyNode{Id: src, Name: src})
	}
	sort.Slice(consumerNodes, func(i, j int) bool {
		return consumerNodes[i].Id < consumerNodes[j].Id
	})

	return &acv1.GetTopologyResponse{
		Producers: producerNodes,
		Consumers: consumerNodes,
		Edges:     edges,
	}, nil
}

func (c *controller) WatchTopology(ctx context.Context, stream *connect.ServerStream[acv1.WatchTopologyResponse]) error {
	obs, cleanup := c.topology.subscribe()
	defer cleanup()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp := <-obs.ch:
			if err := stream.Send(resp); err != nil {
				return err
			}
		}
	}
}

func (c *controller) GetTopicSeries(ctx context.Context, eventType string, windowSecs int32) (*acv1.GetTopicSeriesResponse, error) {
	pts, bucket, err := c.storer.QueryTopicSeries(ctx, eventType, windowSecs)
	if err != nil {
		return nil, err
	}
	protoPoints := make([]*acv1.SeriesPoint, len(pts))
	for i, p := range pts {
		protoPoints[i] = &acv1.SeriesPoint{TimestampMs: p.TimestampMs, Count: p.Count}
	}
	return &acv1.GetTopicSeriesResponse{Points: protoPoints, BucketSecs: bucket}, nil
}

func (c *controller) GetResourceMetrics(_ context.Context) (*acv1.GetResourceMetricsResponse, error) {
	stats := c.storer.StoreStats()
	return &acv1.GetResourceMetricsResponse{
		MessagesPublishedTotal: c.publishedTotal.Load(),
		MessagesDroppedTotal:   stats.DroppedTotal,
		QueueDepth:             stats.QueueDepth,
		QueueCapacity:          stats.QueueCapacity,
		ActiveConsumers:        uint32(c.state.ConsumerCount()),
		ActiveProducers:        uint32(c.topology.ProducerCount()),
		StoreFlushP95Ms:        stats.FlushP95Ms,
		StoreFlushCount:        stats.FlushCount,
	}, nil
}

func (c *controller) GetMetrics(ctx context.Context, windowSecs int32) (*acv1.GetMetricsResponse, error) {
	volumes, err := c.storer.QueryVolumes(ctx, windowSecs)
	if err != nil {
		return nil, err
	}

	latency, err := c.storer.QueryLatency(ctx, windowSecs)
	if err != nil {
		// Non-fatal: return what we have without latency.
		latency = LatencyStats{}
	}

	// Compute total messages/min from the volume counts.
	var totalCount uint32
	for _, v := range volumes {
		totalCount += v.Count
	}
	windowMins := float64(windowSecs) / 60.0
	var msgsPerMin uint32
	if windowMins > 0 {
		msgsPerMin = uint32(float64(totalCount) / windowMins)
	}

	edgeVols := make([]*acv1.EdgeVolume, 0, len(volumes))
	for _, v := range volumes {
		edgeVols = append(edgeVols, &acv1.EdgeVolume{
			Source:    v.Source,
			EventType: v.EventType,
			Count:     v.Count,
		})
	}

	return &acv1.GetMetricsResponse{
		EdgeVolumes:        edgeVols,
		MedianLatencyMs:    latency.MedianMs,
		P95LatencyMs:       latency.P95Ms,
		TotalMessagesPerMin: msgsPerMin,
	}, nil
}
