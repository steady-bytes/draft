package broker

import (
	"context"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	// Given the producer, and consumer will have to share the same in memory data structure to
	// send message to consumers when received from a producer. It's worth keeping the rpc services
	// defined seperatly but implement using the same handler layer
	Rpc interface {
		chassis.RPCRegistrar
		acConnect.ConsumerHandler
		acConnect.ProducerHandler
		acConnect.QueryHandler
		acConnect.TopologyHandler
		acConnect.MetricsHandler
		acConnect.ResourceMetricsHandler
	}

	rpc struct {
		controller Controller
		logger     chassis.Logger
	}
)

func NewRPC(logger chassis.Logger, controller Controller) Rpc {
	return &rpc{
		logger:     logger,
		controller: controller,
	}
}

func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	producerPattern, producerHandler := acConnect.NewProducerHandler(h)
	server.AddHandler(producerPattern, producerHandler, true)

	consumerPattern, consumerHandler := acConnect.NewConsumerHandler(h)
	server.AddHandler(consumerPattern, consumerHandler, true)

	queryPattern, queryHandler := acConnect.NewQueryHandler(h)
	server.AddHandler(queryPattern, queryHandler, true)

	topologyPattern, topologyHandler := acConnect.NewTopologyHandler(h)
	server.AddHandler(topologyPattern, topologyHandler, true)

	metricsPattern, metricsHandler := acConnect.NewMetricsHandler(h)
	server.AddHandler(metricsPattern, metricsHandler, true)

	resourceMetricsPattern, resourceMetricsHandler := acConnect.NewResourceMetricsHandler(h)
	server.AddHandler(resourceMetricsPattern, resourceMetricsHandler, true)
}

// Consume accepts a request containing a `Message` type to subscribe to
// to keep the connection open a `sync.WaitGroup` is created and the response
// stream, and message are passed to the `broker.Consume`
// Since the server stream can only return an error to close the connection
// the `wg.Done()` method is called after the error is logged closing the
// server connection with the client
func (h *rpc) Consume(ctx context.Context, req *connect.Request[acv1.ConsumeRequest], stream *connect.ServerStream[acv1.ConsumeResponse]) error {
	msg := req.Msg.GetMessage()

	if err := h.controller.Consume(ctx, msg, stream); err != nil {
		h.logger.Error(err.Error())
		return err
	}

	<-ctx.Done()

	return ctx.Err()
}

func (h *rpc) Produce(ctx context.Context, inputStream *connect.BidiStream[acv1.ProduceRequest, acv1.ProduceResponse]) error {
	return h.controller.Produce(ctx, inputStream)
}

func (h *rpc) GetTopology(ctx context.Context, _ *connect.Request[acv1.GetTopologyRequest]) (*connect.Response[acv1.GetTopologyResponse], error) {
	resp, err := h.controller.GetTopology(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (h *rpc) WatchTopology(ctx context.Context, _ *connect.Request[acv1.WatchTopologyRequest], stream *connect.ServerStream[acv1.WatchTopologyResponse]) error {
	return h.controller.WatchTopology(ctx, stream)
}

func (h *rpc) GetMetrics(ctx context.Context, req *connect.Request[acv1.GetMetricsRequest]) (*connect.Response[acv1.GetMetricsResponse], error) {
	resp, err := h.controller.GetMetrics(ctx, req.Msg.GetWindowSeconds())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (h *rpc) GetResourceMetrics(ctx context.Context, _ *connect.Request[acv1.GetResourceMetricsRequest]) (*connect.Response[acv1.GetResourceMetricsResponse], error) {
	resp, err := h.controller.GetResourceMetrics(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (h *rpc) GetTopicSeries(ctx context.Context, req *connect.Request[acv1.GetTopicSeriesRequest]) (*connect.Response[acv1.GetTopicSeriesResponse], error) {
	resp, err := h.controller.GetTopicSeries(ctx, req.Msg.GetEventType(), req.Msg.GetWindowSeconds())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

