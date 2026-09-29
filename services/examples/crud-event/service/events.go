package service

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	crudv1 "github.com/steady-bytes/draft/api/examples/crud/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// modelEventType is examples.crud.v1.ModelEvent's fully-qualified proto name -- the CloudEvent
// .Type convention this repo already follows (e.g. pkg/chassis/wide_event.go's wideEventType,
// services/examples/producer/events/events.go's "examples.crud.v1.DatabaseModelSaved").
const modelEventType = "examples.crud.v1.ModelEvent"

type (
	// EventPublisher emits a ModelEvent CloudEvent for one completed repository operation --
	// "completed" meaning the database call already succeeded; Publish reports failures to
	// publish only via logging, never to the caller, the same fire-and-forget posture chassis's
	// own WideEvent producer takes (pkg/chassis/wide_event.go) -- a domain event failing to reach
	// Catalyst is not a reason to fail the RPC whose work already committed.
	EventPublisher interface {
		Publish(ctx context.Context, op crudv1.Operation, model *crudv1.Name)
	}

	eventPublisher struct {
		logger chassis.Logger
		source string
		stream *connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse]
		// mu guards Send: a Connect bidi stream's Send isn't safe for concurrent use, same
		// constraint pkg/chassis/wide_event.go's wideEventSendMu documents for its own singleton
		// stream -- crud-event's handler methods can run concurrently across requests, unlike
		// that package-level singleton, so every publisher instance needs its own lock.
		mu sync.Mutex
	}
)

// NewEventPublisher opens its own Produce stream to Catalyst and returns an EventPublisher that
// sends a ModelEvent over it for every completed repository operation. Client construction
// mirrors services/examples/producer/main.go exactly: same h2c-over-plain-TCP transport
// Catalyst's server needs, same connect.WithGRPC() protocol option.
func NewEventPublisher(ctx context.Context, logger chassis.Logger, catalystAddr, source string) EventPublisher {
	client := acConnect.NewProducerClient(h2cClient(), catalystAddr, connect.WithGRPC())
	return &eventPublisher{
		logger: logger,
		source: source,
		stream: client.Produce(ctx),
	}
}

func (p *eventPublisher) Publish(ctx context.Context, op crudv1.Operation, model *crudv1.Name) {
	data, err := protojson.Marshal(&crudv1.ModelEvent{
		Operation: op,
		Model:     model,
	})
	if err != nil {
		p.logger.WithContext(ctx).WithError(err).Error("failed to marshal ModelEvent")
		return
	}

	event := &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      p.source,
		SpecVersion: "1.0",
		Type:        modelEventType,
		Data:        &acv1.CloudEvent_TextData{TextData: string(data)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{
				CeTimestamp: timestamppb.Now(),
			}},
			"subject": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{
				CeString: model.GetId(),
			}},
		},
	}

	p.mu.Lock()
	err = p.stream.Send(&acv1.ProduceRequest{Message: event})
	p.mu.Unlock()
	if err != nil {
		p.logger.WithContext(ctx).WithError(err).WithField("operation", op.String()).Error("failed to publish ModelEvent")
	}
}

// h2cClient returns an HTTP client that speaks HTTP/2 cleartext (h2c), required for gRPC
// streaming against Catalyst's plain-TCP server -- copied verbatim from
// services/examples/producer/main.go's own helper of the same name/shape.
func h2cClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}
