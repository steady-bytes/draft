package service

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	relayv1 "github.com/steady-bytes/draft/api/tooling/relay/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// transcriptSegmentEventType is tooling.relay.v1.TranscriptSegment's fully-qualified proto name,
// used as the CloudEvent .Type -- the same convention services/examples/crud-event/service/
// events.go's modelEventType follows. See the implementation plan's Telemetry section: each
// finalized TranscriptSegment produces one of these, independent of the WideEvent span covering
// the RPC/pipeline step that finalized it.
const transcriptSegmentEventType = "tooling.relay.v1.TranscriptSegment"

type (
	// EventPublisher emits a relay.transcript.segment-shaped CloudEvent for each finalized
	// transcript segment. Fire-and-forget, same posture as crud-event's own EventPublisher: a
	// domain event failing to reach Catalyst is never a reason to fail the recording session that
	// produced it.
	EventPublisher interface {
		PublishTranscriptSegment(ctx context.Context, seg *relayv1.TranscriptSegment)
	}

	eventPublisher struct {
		logger chassis.Logger
		source string
		stream *connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse]
		// mu guards Send -- a Connect bidi stream's Send isn't safe for concurrent use, same
		// constraint crud-event's own eventPublisher documents.
		mu sync.Mutex
	}
)

// NewEventPublisher opens its own Produce stream to Catalyst, mirroring
// services/examples/crud-event/service/events.go's NewEventPublisher exactly (same h2c transport,
// same connect.WithGRPC() option).
func NewEventPublisher(ctx context.Context, logger chassis.Logger, catalystAddr, source string) EventPublisher {
	client := acConnect.NewProducerClient(h2cClient(), catalystAddr, connect.WithGRPC())
	return &eventPublisher{
		logger: logger,
		source: source,
		stream: client.Produce(ctx),
	}
}

func (p *eventPublisher) PublishTranscriptSegment(ctx context.Context, seg *relayv1.TranscriptSegment) {
	data, err := protojson.Marshal(seg)
	if err != nil {
		p.logger.WithContext(ctx).WithError(err).Error("failed to marshal TranscriptSegment")
		return
	}

	event := &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      p.source,
		SpecVersion: "1.0",
		Type:        transcriptSegmentEventType,
		Data:        &acv1.CloudEvent_TextData{TextData: string(data)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{
				CeTimestamp: timestamppb.Now(),
			}},
			"subject": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{
				CeString: seg.GetRecordingId(),
			}},
		},
	}

	p.mu.Lock()
	err = p.stream.Send(&acv1.ProduceRequest{Message: event})
	p.mu.Unlock()
	if err != nil {
		p.logger.WithContext(ctx).WithError(err).WithField("recording_id", seg.GetRecordingId()).Error("failed to publish TranscriptSegment event")
	}
}

// h2cClient returns an HTTP client that speaks HTTP/2 cleartext (h2c), required for gRPC
// streaming against Catalyst's plain-TCP server -- copied verbatim from
// services/examples/crud-event/service/events.go's own helper of the same name/shape.
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
