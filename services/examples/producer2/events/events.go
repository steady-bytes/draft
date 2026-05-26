package events

import (
	"math/rand/v2"

	"github.com/google/uuid"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	echov1 "github.com/steady-bytes/draft/api/examples/echo/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var senders = []string{
	"/services/gateway",
	"/services/scheduler",
	"/services/cli",
	"/services/webhook",
	"/services/mobile-app",
}

var greetings = []string{
	"Hello from the other side",
	"Greetings, world!",
	"Hi there",
	"Hey!",
	"Salutations",
	"Howdy",
	"What's up",
}

// Random returns a randomly-selected CloudEvent. Distribution: 40% HelloWorld,
// 30% Ping, 30% Pong.
func Random() (*acv1.CloudEvent, error) {
	n := rand.IntN(10)
	switch {
	case n < 4:
		return RandomHelloWorld()
	case n < 7:
		return RandomPing()
	default:
		return RandomPong()
	}
}

func RandomHelloWorld() (*acv1.CloudEvent, error) {
	sender := senders[rand.IntN(len(senders))]
	msg := greetings[rand.IntN(len(greetings))]
	return NewHelloWorld(uuid.NewString(), msg, sender)
}

func RandomPing() (*acv1.CloudEvent, error) {
	sender := senders[rand.IntN(len(senders))]
	return NewPing(uuid.NewString(), sender)
}

func RandomPong() (*acv1.CloudEvent, error) {
	responder := senders[rand.IntN(len(senders))]
	return NewPong(uuid.NewString(), uuid.NewString(), responder)
}

func NewHelloWorld(id, message, sender string) (*acv1.CloudEvent, error) {
	b, err := protojson.Marshal(&echov1.HelloWorld{
		Message: message,
		Sender:  sender,
	})
	if err != nil {
		return nil, err
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      sender,
		SpecVersion: "1.0",
		Type:        "examples.echo.v1.HelloWorld",
		Data:        &acv1.CloudEvent_TextData{TextData: string(b)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    timeAttr(),
			"subject": subjectAttr(id),
		},
	}, nil
}

func NewPing(id, sender string) (*acv1.CloudEvent, error) {
	b, err := protojson.Marshal(&echov1.Ping{
		Id:     id,
		Sender: sender,
	})
	if err != nil {
		return nil, err
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      sender,
		SpecVersion: "1.0",
		Type:        "examples.echo.v1.Ping",
		Data:        &acv1.CloudEvent_TextData{TextData: string(b)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    timeAttr(),
			"subject": subjectAttr(id),
		},
	}, nil
}

func NewPong(id, pingID, responder string) (*acv1.CloudEvent, error) {
	b, err := protojson.Marshal(&echov1.Pong{
		PingId:    pingID,
		Responder: responder,
	})
	if err != nil {
		return nil, err
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      responder,
		SpecVersion: "1.0",
		Type:        "examples.echo.v1.Pong",
		Data:        &acv1.CloudEvent_TextData{TextData: string(b)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    timeAttr(),
			"subject": subjectAttr(id),
		},
	}, nil
}

func timeAttr() *acv1.CloudEvent_CloudEventAttributeValue {
	return &acv1.CloudEvent_CloudEventAttributeValue{
		Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{
			CeTimestamp: timestamppb.Now(),
		},
	}
}

func subjectAttr(s string) *acv1.CloudEvent_CloudEventAttributeValue {
	return &acv1.CloudEvent_CloudEventAttributeValue{
		Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{
			CeString: s,
		},
	}
}
