// This file implements Phase 7: publishing a CloudEvent on Catalyst for every
// run/step status transition, per the doc's "webhook trigger and status API"
// section -- "every status transition ... is also published as a CloudEvent
// on Catalyst ... [so] Bench's own UI [can] update live instead of
// re-polling on a timer." Uses the same raw Connect bidi-stream Producer
// client services/examples/producer/main.go demonstrates: Catalyst's
// chassis.Broker interface (pkg/chassis/broker.go) is what a broker
// *implementation* (NATS, AMQP) backs, not something an ordinary client
// publishes through -- a service that wants to emit events onto Catalyst
// talks to its ProducerService like any other Connect client.
package main

import (
	"context"
	"fmt"
	"sync"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// defaultCatalystAddress matches services/examples/producer/config.yaml's own
// fallback -- the same "static config, not Blueprint-resolved" convention
// that service's main.go uses for reaching Catalyst.
const defaultCatalystAddress = "http://localhost:2220"

// EventPublisher is how the scheduler reports run/step status transitions --
// see scheduler.go's calls in execute/runStep. A separate interface (rather
// than the scheduler depending on *catalystPublisher directly) so tests can
// run against a no-op instead of a real Catalyst connection, the same
// boundary discipline Executor and ServiceResolver already use elsewhere in
// this package.
type EventPublisher interface {
	RunStarted(run *workflowv1.Run)
	RunFinished(run *workflowv1.Run)
	StepCompleted(runID, workflowName string, step *workflowv1.StepResult)
}

// noopEventPublisher is the default wired into newSchedulerWithExecutors
// (used directly by every scheduler_test.go case) and anywhere a real
// Catalyst connection isn't available or desired.
type noopEventPublisher struct{}

func (noopEventPublisher) RunStarted(*workflowv1.Run)                           {}
func (noopEventPublisher) RunFinished(*workflowv1.Run)                          {}
func (noopEventPublisher) StepCompleted(string, string, *workflowv1.StepResult) {}

// catalystPublisher is the real EventPublisher, backed by a single long-lived
// Produce bidi-stream to Catalyst opened once and reused for the life of the
// process -- the same pattern services/examples/producer/main.go uses for its
// own event stream (a Connect bidi stream doesn't dial until the first Send,
// so construction here is cheap even if Catalyst isn't up yet).
type catalystPublisher struct {
	logger chassis.Logger
	mu     sync.Mutex
	stream *connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse]
}

// NewCatalystPublisher opens the Produce stream against catalyst.address
// (config), defaulting to defaultCatalystAddress. ctx should outlive every
// call this publisher will ever make -- main.go passes one tied to
// chassis.Closer(), so the stream is torn down on shutdown, not per-call.
func NewCatalystPublisher(ctx context.Context, httpClient connect.HTTPClient, cfg chassis.Config, logger chassis.Logger) *catalystPublisher {
	addr := cfg.GetString("catalyst.address")
	if addr == "" {
		addr = defaultCatalystAddress
	}
	client := acConnect.NewProducerClient(httpClient, addr, connect.WithGRPC())
	return &catalystPublisher{logger: logger, stream: client.Produce(ctx)}
}

func (p *catalystPublisher) RunStarted(run *workflowv1.Run) {
	p.publish(runEvent("tooling.workflow.v1.RunStarted", run))
}

func (p *catalystPublisher) RunFinished(run *workflowv1.Run) {
	p.publish(runEvent("tooling.workflow.v1.RunFinished", run))
}

func (p *catalystPublisher) StepCompleted(runID, workflowName string, step *workflowv1.StepResult) {
	p.publish(stepEvent(runID, workflowName, step))
}

// publish sends event on the shared stream, best-effort: a publish failure
// (Catalyst unreachable, stream broken) is logged, not surfaced further --
// the same "best-effort, not fatal" convention already established for
// persistence throughout this package (see runStep's UpsertStepResult
// comment). A subscriber that misses an event can always fall back to
// GetRun, which remains the source of truth per the doc. err is the result
// of building event (a marshal failure); accepted alongside event so every
// call site can stay a single expression (p.publish(runEvent(...))) rather
// than an if-err block at each of the three call sites above.
func (p *catalystPublisher) publish(event *acv1.CloudEvent, err error) {
	if err != nil {
		p.logger.WithError(err).Error("failed to build catalyst event")
		return
	}
	// A Connect bidi stream's Send is not safe for concurrent use, and the
	// scheduler runs steps concurrently (goroutine-per-step, scheduler.go's
	// execute) -- without this lock, two events firing at once would race on
	// the same underlying HTTP/2 stream.
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.stream.Send(&acv1.ProduceRequest{Message: event}); err != nil {
		p.logger.WithError(err).Warn("failed to publish catalyst event")
	}
}

func runEvent(eventType string, run *workflowv1.Run) (*acv1.CloudEvent, error) {
	body, err := protojson.Marshal(&workflowv1.RunEvent{
		RunId:        run.GetRunId(),
		WorkflowName: run.GetWorkflowName(),
		Status:       run.GetStatus(),
		StartedAt:    run.GetStartedAt(),
		FinishedAt:   run.GetFinishedAt(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal run event: %w", err)
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      "/services/bench",
		SpecVersion: "1.0",
		Type:        eventType,
		Data:        &acv1.CloudEvent_TextData{TextData: string(body)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    ceTimeAttr(),
			"subject": ceStringAttr(run.GetRunId()),
		},
	}, nil
}

func stepEvent(runID, workflowName string, step *workflowv1.StepResult) (*acv1.CloudEvent, error) {
	body, err := protojson.Marshal(&workflowv1.StepEvent{
		RunId:        runID,
		WorkflowName: workflowName,
		StepName:     step.GetStepName(),
		Status:       step.GetStatus(),
		Error:        step.GetError(),
		StartedAt:    step.GetStartedAt(),
		FinishedAt:   step.GetFinishedAt(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal step event: %w", err)
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      "/services/bench",
		SpecVersion: "1.0",
		Type:        "tooling.workflow.v1.StepCompleted",
		Data:        &acv1.CloudEvent_TextData{TextData: string(body)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    ceTimeAttr(),
			"subject": ceStringAttr(runID),
		},
	}, nil
}

func ceTimeAttr() *acv1.CloudEvent_CloudEventAttributeValue {
	return &acv1.CloudEvent_CloudEventAttributeValue{
		Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{CeTimestamp: timestamppb.Now()},
	}
}

func ceStringAttr(s string) *acv1.CloudEvent_CloudEventAttributeValue {
	return &acv1.CloudEvent_CloudEventAttributeValue{
		Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{CeString: s},
	}
}
