package chassis

import (
	"context"

	events "github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
)

type (
	Broker interface {
		Open(ctx context.Context, config Config) error
		Publish(ctx context.Context, options PublishOptions) error
		Subscribe(ctx context.Context, options SubscribeOptions) error
		Unsubscribe(ctx context.Context, options UnsubscribeOptions) error
		Close(force bool) error
	}
	Consumer interface {
		Consume(ctx context.Context, event *events.CloudEvent) error
	}
	PublishOptions struct {
		Event *events.CloudEvent
		// Tags are an optional list of strings that can be used to contextualize the event. For example, a tag could be used in
		// a blue/green deployment scenario to indicate which version of the application published the event. Keep in mind that tags
		// are only *contextual* data and though they do affect the routing of the event, they do not affect the content of the event.
		// For a consumer to receive an event, it must have a matching event type and all tags that were used when the event was published.
		// Tags will be sorted alphabetically before being used to route the event so that the order of the tags does not matter.
		Tags []string
	}
	SubscribeOptions struct {
		Event *events.CloudEvent
		// Tags are an optional list of strings that can be used to filter the events that are received. For example, a tag could be used
		// in a blue/green deployment scenario to filter out events that were published by the other version of the application. If no tags
		// are provided, all events that match the provided Event will be received.
		Tags     []string
		Consumer Consumer
		// Group is the name of the group the consumer belongs to. If not empty, messages delivered to the group
		// will be load-balanced across all consumers within the group. If empty, the consumer will receive all
		// messages that match the provided Event and Tags.
		//
		// WARNING: Make sure that this value is globally unique across all other groups that are consuming the same event. Otherwise
		// messages could be delivered to consumers unintentionally.
		Group string
		// IgnoreType states whether the subscribing consumer should ignore the event type when receiving messages. If true, the consumer
		// will receive all events that match Event.Source and any provided Tags. If false, the consumer will only receive events that match
		// Event.Source, Event.Type, and any provided Tags.
		IgnoreType bool
	}
	UnsubscribeOptions struct {
		Event      *events.CloudEvent
		Tags       []string
		IgnoreType bool
	}
)
