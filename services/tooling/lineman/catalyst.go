// catalyst.go: publishes one CloudEvent per state change, for downstream
// consumption -- see docs/architecture/lineman-implementation-plan.md's
// Telemetry section. Mirrors services/tooling/bench/catalyst.go's
// long-lived-stream pattern and CloudEvent envelope convention exactly.
package main

import (
	"context"
	"fmt"
	"sync"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultCatalystAddress = "http://localhost:2220"

// EventPublisher lets the rest of this service depend on an interface
// instead of *catalystPublisher directly -- a noop implementation is used
// wherever a real Catalyst connection isn't available (tests).
type EventPublisher interface {
	TaskStateChanged(t *linemanv1.Task, fromState string)
	ScheduledTaskFired(s *linemanv1.ScheduledTask, taskID string)
	LoopFired(l *linemanv1.Loop, taskID string)
}

type noopEventPublisher struct{}

func (noopEventPublisher) TaskStateChanged(*linemanv1.Task, string)            {}
func (noopEventPublisher) ScheduledTaskFired(*linemanv1.ScheduledTask, string) {}
func (noopEventPublisher) LoopFired(*linemanv1.Loop, string)                   {}

type catalystPublisher struct {
	logger chassis.Logger
	mu     sync.Mutex
	stream *connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse]
}

// NewCatalystPublisher opens the Produce stream against catalyst.address
// (config), defaulting to defaultCatalystAddress. ctx should outlive every
// call this publisher will ever make -- main.go passes one tied to
// chassis.Closer().
func NewCatalystPublisher(ctx context.Context, httpClient connect.HTTPClient, cfg chassis.Config, logger chassis.Logger) EventPublisher {
	addr := cfg.GetString("catalyst.address")
	if addr == "" {
		addr = defaultCatalystAddress
	}
	client := acConnect.NewProducerClient(httpClient, addr, connect.WithGRPC())
	return &catalystPublisher{logger: logger, stream: client.Produce(ctx)}
}

func (p *catalystPublisher) TaskStateChanged(t *linemanv1.Task, fromState string) {
	p.publish(buildEvent("tooling.lineman.v1.TaskStateChanged", t.GetId(), t, map[string]string{
		"objective_id": t.GetObjectiveId(),
		"from_state":   fromState,
		"to_state":     t.GetState(),
	}))
}

func (p *catalystPublisher) ScheduledTaskFired(s *linemanv1.ScheduledTask, taskID string) {
	p.publish(buildEvent("tooling.lineman.v1.ScheduledTaskFired", s.GetId(), s, map[string]string{
		"created_task_id": taskID,
	}))
}

func (p *catalystPublisher) LoopFired(l *linemanv1.Loop, taskID string) {
	p.publish(buildEvent("tooling.lineman.v1.LoopFired", l.GetId(), l, map[string]string{
		"created_task_id": taskID,
	}))
}

// publish sends event on the shared stream, best-effort -- same convention
// as services/tooling/bench/catalyst.go's own publish: a failure to build or
// send an event is logged, never surfaced as an RPC error. This is an
// observability side-channel; Blueprint's stored state (not this stream)
// remains the source of truth.
func (p *catalystPublisher) publish(event *acv1.CloudEvent, err error) {
	if err != nil {
		p.logger.WithError(err).Error("failed to build catalyst event")
		return
	}
	// A Connect bidi stream's Send is not safe for concurrent use.
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.stream.Send(&acv1.ProduceRequest{Message: event}); err != nil {
		p.logger.WithError(err).Warn("failed to publish catalyst event")
	}
}

// buildEvent matches the CloudEvent envelope convention documented in
// docs/architecture/wide-events.md's "CloudEvent envelope" table and used by
// every real producer in this repo: id = a fresh uuid, source = a path-like
// identity for this service, type = package + message name, data =
// protojson-marshaled payload. extraAttrs become CloudEvent string
// attributes, alongside a "subject" (subjectID) and "time" attribute set on
// every event.
func buildEvent(eventType, subjectID string, payload proto.Message, extraAttrs map[string]string) (*acv1.CloudEvent, error) {
	body, err := protojson.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %T: %w", payload, err)
	}
	attrs := map[string]*acv1.CloudEvent_CloudEventAttributeValue{
		"time":    {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{CeTimestamp: timestamppb.Now()}},
		"subject": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{CeString: subjectID}},
	}
	for k, v := range extraAttrs {
		attrs[k] = &acv1.CloudEvent_CloudEventAttributeValue{Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{CeString: v}}
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      "/services/lineman",
		SpecVersion: "1.0",
		Type:        eventType,
		Data:        &acv1.CloudEvent_TextData{TextData: string(body)},
		Attributes:  attrs,
	}, nil
}
