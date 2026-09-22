// type_mutation_consumer.go implements Blueprint's first-ever dependency on Catalyst: a single,
// generic Consume stream that lets any process with a type already registered via RegisterType
// mutate values of that type over Catalyst instead of a direct Set/Delete RPC. See
// docs/website/content/docs/architecture/core-services.md's "Type Mutation Events" section.
//
// This is deliberately generic -- the consumer has no per-type logic. It resolves an incoming
// TypeMutation's value.type_url against the same registered-type cache DecodeValues already uses
// (typeRegistry.lookup) purely as an allowlist check: only a type that has actually been
// registered can be mutated this way, the same trust boundary RegisterType already establishes
// for decoding. The mutation itself never needs the resolved descriptor -- TypeMutation.Value is
// already a *anypb.Any, the exact shape Set/Delete/Get take as T (see model.go), so it's passed
// straight through to the same controller methods the RPC handlers call.
package key_value

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
)

// typeMutationEventType is the CloudEvent type TypeMutation events are produced/consumed under.
// Matches wide_event.go's wideEventType convention: the message's own fully-qualified proto name.
const typeMutationEventType = "core.registry.key_value.v1.TypeMutation"

// typeMutationReconnectDelay is how long the consumer waits before retrying after its Consume
// stream ends, for any reason -- including the expected case of Catalyst simply not being up yet
// (Blueprint starts *before* Catalyst in cluster startup order -- see
// docs/website/content/docs/architecture/core-services.md -- so the very first attempt is
// expected to fail every time in a freshly started cluster) and the unexpected case of Catalyst
// restarting later on. Unlike chassis's wide_event.go producer (a known, accepted, not-yet-fixed
// limitation -- see that file's doc comment), this consumer reconnects for the lifetime of the
// process rather than giving up after one failure, since nothing else retries this on its behalf.
const typeMutationReconnectDelay = 2 * time.Second

// defaultCatalystAddress matches wide_event.go's and catalyst-consume/rpc.go's own fallback.
const defaultCatalystAddress = "http://localhost:2220"

// StartTypeMutationConsumer runs the generic TypeMutation consume loop forever, reconnecting with
// backoff whenever the stream ends. Intended to be run via chassis.Runtime.WithRunner, which
// already puts it in its own goroutine -- see main.go.
func StartTypeMutationConsumer(logger chassis.Logger, ctrl Controller) {
	log := logger.WithField("component", "type_mutation_consumer")

	for {
		if err := consumeTypeMutations(log, ctrl); err != nil {
			log.WithError(err).Debug("type mutation consume stream ended, reconnecting")
		}
		time.Sleep(typeMutationReconnectDelay)
	}
}

// consumeTypeMutations opens one Consume stream and processes events from it until the stream
// ends (Catalyst not yet up, a network error, or Catalyst restarting), returning that error to
// the caller's reconnect loop.
func consumeTypeMutations(log chassis.Logger, ctrl Controller) error {
	catalystAddr := chassis.GetConfig().GetString("catalyst.address")
	if catalystAddr == "" {
		catalystAddr = defaultCatalystAddress
	}

	client := acConnect.NewConsumerClient(typeMutationH2CClient(), catalystAddr, connect.WithGRPC())
	stream, err := client.Consume(context.Background(), connect.NewRequest(&acv1.ConsumeRequest{
		Message: &acv1.CloudEvent{
			Source: "blueprint",
			Type:   typeMutationEventType,
		},
	}))
	if err != nil {
		return err
	}

	for stream.Receive() {
		event := stream.Msg().GetMessage()
		if event == nil || event.GetType() != typeMutationEventType {
			continue
		}
		applyTypeMutationEvent(log, ctrl, event)
	}

	return stream.Err()
}

// applyTypeMutationEvent decodes one CloudEvent's proto_data into a TypeMutation and applies it.
// Never returns an error -- a malformed event or an unregistered type_url is logged and dropped,
// matching this whole mechanism's fire-and-forget contract (see this file's doc comment).
func applyTypeMutationEvent(log chassis.Logger, ctrl Controller, event *acv1.CloudEvent) {
	mutation := &kvv1.TypeMutation{}
	if err := event.GetProtoData().UnmarshalTo(mutation); err != nil {
		log.WithError(err).Warn("dropping type mutation event: failed to decode proto_data")
		return
	}

	value := mutation.GetValue()
	log = log.WithField("key", mutation.GetKey()).
		WithField("action", mutation.GetAction().String()).
		WithField("type_url", value.GetTypeUrl())

	if !ctrl.TypeRegistered(value.GetTypeUrl()) {
		log.Warn("dropping type mutation event: type_url is not registered with blueprint")
		return
	}

	// Self-initiated, not on behalf of any external RPC caller -- attributed to "blueprint" the
	// same way LeadershipChange attributes its own writes (controller.go).
	ctx := WithCallerService(context.Background(), "blueprint.type_mutation_consumer")

	switch mutation.GetAction() {
	case kvv1.TypeMutation_ACTION_CREATE, kvv1.TypeMutation_ACTION_UPDATE:
		if _, err := ctrl.Set(ctx, log, mutation.GetKey(), value, 500*time.Millisecond); err != nil {
			log.WithError(err).Warn("dropping type mutation event: failed to apply set")
		}
	case kvv1.TypeMutation_ACTION_DELETE:
		if err := ctrl.Delete(ctx, log, mutation.GetKey(), value, 500*time.Millisecond); err != nil {
			log.WithError(err).Warn("dropping type mutation event: failed to apply delete")
		}
	default:
		log.Warn("dropping type mutation event: unspecified action")
	}
}

// typeMutationH2CClient returns an HTTP client that speaks HTTP/2 cleartext (h2c), required for
// Connect streaming against Catalyst's plain-TCP server -- same construction every other
// Producer/Consumer client in this repo uses (wide_event.go, catalyst-consume/rpc.go).
func typeMutationH2CClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}
