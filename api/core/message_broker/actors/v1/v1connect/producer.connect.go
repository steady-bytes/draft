// Code generated by protoc-gen-connect-go. DO NOT EDIT.
//
// Source: core/message_broker/actors/v1/producer.proto

package v1connect

import (
	connect "connectrpc.com/connect"
	context "context"
	errors "errors"
	v1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	http "net/http"
	strings "strings"
)

// This is a compile-time assertion to ensure that this generated file and the connect package are
// compatible. If you get a compiler error that this constant is not defined, this code was
// generated with a version of connect newer than the one compiled into your binary. You can fix the
// problem by either regenerating this code with an older version of connect or updating the connect
// version compiled into your binary.
const _ = connect.IsAtLeastVersion1_13_0

const (
	// ProducerName is the fully-qualified name of the Producer service.
	ProducerName = "core.message_broker.actors.v1.Producer"
)

// These constants are the fully-qualified names of the RPCs defined in this package. They're
// exposed at runtime as Spec.Procedure and as the final two segments of the HTTP route.
//
// Note that these are different from the fully-qualified method names used by
// google.golang.org/protobuf/reflect/protoreflect. To convert from these constants to
// reflection-formatted method names, remove the leading slash and convert the remaining slash to a
// period.
const (
	// ProducerProduceProcedure is the fully-qualified name of the Producer's Produce RPC.
	ProducerProduceProcedure = "/core.message_broker.actors.v1.Producer/Produce"
)

// These variables are the protoreflect.Descriptor objects for the RPCs defined in this package.
var (
	producerServiceDescriptor       = v1.File_core_message_broker_actors_v1_producer_proto.Services().ByName("Producer")
	producerProduceMethodDescriptor = producerServiceDescriptor.Methods().ByName("Produce")
)

// ProducerClient is a client for the core.message_broker.actors.v1.Producer service.
type ProducerClient interface {
	Produce(context.Context) *connect.BidiStreamForClient[v1.ProduceRequest, v1.ProduceResponse]
}

// NewProducerClient constructs a client for the core.message_broker.actors.v1.Producer service. By
// default, it uses the Connect protocol with the binary Protobuf Codec, asks for gzipped responses,
// and sends uncompressed requests. To use the gRPC or gRPC-Web protocols, supply the
// connect.WithGRPC() or connect.WithGRPCWeb() options.
//
// The URL supplied here should be the base URL for the Connect or gRPC server (for example,
// http://api.acme.com or https://acme.com/grpc).
func NewProducerClient(httpClient connect.HTTPClient, baseURL string, opts ...connect.ClientOption) ProducerClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &producerClient{
		produce: connect.NewClient[v1.ProduceRequest, v1.ProduceResponse](
			httpClient,
			baseURL+ProducerProduceProcedure,
			connect.WithSchema(producerProduceMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
	}
}

// producerClient implements ProducerClient.
type producerClient struct {
	produce *connect.Client[v1.ProduceRequest, v1.ProduceResponse]
}

// Produce calls core.message_broker.actors.v1.Producer.Produce.
func (c *producerClient) Produce(ctx context.Context) *connect.BidiStreamForClient[v1.ProduceRequest, v1.ProduceResponse] {
	return c.produce.CallBidiStream(ctx)
}

// ProducerHandler is an implementation of the core.message_broker.actors.v1.Producer service.
type ProducerHandler interface {
	Produce(context.Context, *connect.BidiStream[v1.ProduceRequest, v1.ProduceResponse]) error
}

// NewProducerHandler builds an HTTP handler from the service implementation. It returns the path on
// which to mount the handler and the handler itself.
//
// By default, handlers support the Connect, gRPC, and gRPC-Web protocols with the binary Protobuf
// and JSON codecs. They also support gzip compression.
func NewProducerHandler(svc ProducerHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	producerProduceHandler := connect.NewBidiStreamHandler(
		ProducerProduceProcedure,
		svc.Produce,
		connect.WithSchema(producerProduceMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	return "/core.message_broker.actors.v1.Producer/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case ProducerProduceProcedure:
			producerProduceHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// UnimplementedProducerHandler returns CodeUnimplemented from all methods.
type UnimplementedProducerHandler struct{}

func (UnimplementedProducerHandler) Produce(context.Context, *connect.BidiStream[v1.ProduceRequest, v1.ProduceResponse]) error {
	return connect.NewError(connect.CodeUnimplemented, errors.New("core.message_broker.actors.v1.Producer.Produce is not implemented"))
}
