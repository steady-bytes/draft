package nats

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/steady-bytes/draft/pkg/chassis"

	events "github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

type (
	broker struct {
		connection    *nats.Conn
		configKey     string
		subscriptions map[string]*nats.Subscription
	}
)

// New instantiates a new broker. A call to Open is required before use.
// The configKey parameter dictates which key in the configuration will be read during
// initialization. Default: "repositories.nats"
func New(configKey string) chassis.Broker {
	if configKey == "" {
		configKey = "brokers.nats"
	}
	return &broker{
		configKey:     configKey,
		subscriptions: make(map[string]*nats.Subscription),
	}
}

func (b *broker) Open(ctx context.Context, config chassis.Config) error {
	url := config.GetString(fmt.Sprintf("%s.url", b.configKey))
	nc, err := nats.Connect(url)
	if err != nil {
		return err
	}
	b.connection = nc
	return nil
}

func (b *broker) Publish(ctx context.Context, options chassis.PublishOptions) error {
	// serialize the event
	data, err := proto.Marshal(options.Event)
	if err != nil {
		return err
	}
	// publish the event
	err = b.connection.Publish(subject(options.Event, false, options.Tags...), data)
	if err != nil {
		return err
	}
	return nil
}

func (b *broker) Subscribe(ctx context.Context, options chassis.SubscribeOptions) error {
	if options.Group != "" {
		return b.queueSubscribe(ctx, options)
	}
	return b.subscribe(ctx, options)
}

// subscribe creates a subscription to the given event and tags
func (b *broker) subscribe(ctx context.Context, options chassis.SubscribeOptions) error {
	// subscribe to the subject
	subscription, err := b.connection.Subscribe(subject(options.Event, options.IgnoreType, options.Tags...), func(msg *nats.Msg) {
		// deserialize the event
		event := &events.CloudEvent{}
		err := proto.Unmarshal(msg.Data, event)
		if err != nil {
			msg.Nak()
		}
		// consume the event
		err = options.Consumer.Consume(ctx, event)
		if err != nil {
			msg.Nak()
		}
		msg.Ack()
	})
	if err != nil {
		return err
	}
	b.subscriptions[subject(options.Event, options.IgnoreType, options.Tags...)] = subscription
	return nil
}

// queueSubscribe creates a subscription to the given event and tags using a queue group
func (b *broker) queueSubscribe(ctx context.Context, options chassis.SubscribeOptions) error {
	// subscribe to the subject
	subscription, err := b.connection.QueueSubscribe(subject(options.Event, options.IgnoreType, options.Tags...), options.Group, func(msg *nats.Msg) {
		// deserialize the event
		event := &events.CloudEvent{}
		err := proto.Unmarshal(msg.Data, event)
		if err != nil {
			msg.Nak()
		}
		// consume the event
		err = options.Consumer.Consume(ctx, event)
		if err != nil {
			msg.Nak()
		}
		msg.Ack()
	})
	if err != nil {
		return err
	}
	b.subscriptions[subject(options.Event, options.IgnoreType, options.Tags...)] = subscription
	return nil
}

func (b *broker) Unsubscribe(ctx context.Context, options chassis.UnsubscribeOptions) error {
	subscription, ok := b.subscriptions[subject(options.Event, options.IgnoreType, options.Tags...)]
	if !ok {
		return fmt.Errorf("no subscription found for event")
	}
	err := subscription.Unsubscribe()
	if err != nil {
		return err
	}
	delete(b.subscriptions, subject(options.Event, options.IgnoreType, options.Tags...))
	return nil
}

func (b *broker) Close(force bool) error {
	if force {
		b.connection.Close()
		return nil
	}
	return b.connection.Drain()
}

// HELPERS

// subject returns the subject for the given event and tags. The format will always follow the pattern:
//
// If no tags are provided and ignoreType is true: {event.Source}.>
// If no tags are provided and ignoreType is false: {event.Source}.{event.Type}.>
// If tags are provided and ignoreType is true: {event.Source}.*.{tags}
// If tags are provided and ignoreType is false: {event.Source}.{event.Type}.{tags}
//
// And the tags will be sorted alphabetically.
func subject(event *events.CloudEvent, ignoreType bool, tags ...string) string {
	if len(tags) == 0 {
		if ignoreType {
			return fmt.Sprintf("%s.>", event.Source)
		}
		return fmt.Sprintf("%s.%s.>", event.Source, event.Type)
	}
	sort.Strings(tags)
	if ignoreType {
		return fmt.Sprintf("%s.*.%s", event.Source, strings.Join(tags, "."))
	}
	return fmt.Sprintf("%s.%s.%s", event.Source, event.Type, strings.Join(tags, "."))
}
