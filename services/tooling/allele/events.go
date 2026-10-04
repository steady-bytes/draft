// events.go publishes Allele's own Catalyst CloudEvents -- see the plan's Telemetry section for the
// type/source convention each method follows. Mirrors services/examples/crud-event/service/events.go
// and services/tooling/lineman/catalyst.go's long-lived-stream pattern, with one deliberate
// departure: the h2c transport is built explicitly (h2cClient, below), the way crud-event's own
// publisher does, rather than handed a plain http.DefaultClient the way lineman's is -- a bare
// http.Client has no h2c support, and connect.WithGRPC() needs real HTTP/2 framing (bidi streaming,
// trailers-based status) to function at all against a plain-TCP (non-TLS) Catalyst address.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultCatalystAddress = "http://localhost:2220"

// EventPublisher is Allele's own CloudEvent emission surface -- source strings follow the dynamic
// prefix+id convention documented in agents.md's #cloudevent-mapping (the precedent the plan's own
// Telemetry section cites), not a single fixed per-service source. A noopEventPublisher (matching
// lineman's own EventPublisher/noop pair) backs any path without a real Catalyst connection, so
// store methods can call these unconditionally rather than nil-checking everywhere.
//
// LinkageChanged and MergeCompleted, also listed in Telemetry, have no interface methods here --
// nothing yet computes a linkage manifest (Phase 11) or actually executes a queued merge (no phase
// has built that mechanism; see this phase's own completion note), so there is nothing real to fire
// either from. Adding unused methods "for completeness" would be event shapes nobody could verify.
type EventPublisher interface {
	SymbolChanged(repositoryID, worktreeID string, change *allelev1.SymbolChange)
	ConflictDetected(repositoryID, changeID string, conflict *allelev1.ConflictReport)
	VerificationCompleted(repositoryID string, run *allelev1.VerificationRun)
}

type noopEventPublisher struct{}

func (noopEventPublisher) SymbolChanged(string, string, *allelev1.SymbolChange)      {}
func (noopEventPublisher) ConflictDetected(string, string, *allelev1.ConflictReport) {}
func (noopEventPublisher) VerificationCompleted(string, *allelev1.VerificationRun)   {}

type catalystPublisher struct {
	logger chassis.Logger
	mu     sync.Mutex
	stream *connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse]
}

// NewCatalystPublisher opens the Produce stream against catalyst.address (config), defaulting to
// defaultCatalystAddress. ctx should outlive every call this publisher will ever make -- main.go
// ties it to chassis.Closer(), the same convention crud-event/lineman both use. Known, pre-existing,
// shared limitation (not specific to this publisher): restarting Catalyst breaks this stream with no
// automatic reconnect, same as every other long-lived Produce stream in this repo today.
func NewCatalystPublisher(ctx context.Context, logger chassis.Logger, catalystAddr string) EventPublisher {
	if catalystAddr == "" {
		catalystAddr = defaultCatalystAddress
	}
	client := acConnect.NewProducerClient(h2cClient(), catalystAddr, connect.WithGRPC())
	return &catalystPublisher{logger: logger, stream: client.Produce(ctx)}
}

func (p *catalystPublisher) SymbolChanged(repositoryID, worktreeID string, change *allelev1.SymbolChange) {
	p.publish(buildEvent("tooling.allele.v1.SymbolChanged", "allele.SymbolChange/"+repositoryID, change.GetId(),
		&allelev1.SymbolChangeEvent{
			RepositoryId: repositoryID,
			WorktreeId:   worktreeID,
			Change:       change,
			At:           timestamppb.Now(),
		}))
}

func (p *catalystPublisher) ConflictDetected(repositoryID, changeID string, conflict *allelev1.ConflictReport) {
	p.publish(buildEvent("tooling.allele.v1.ConflictDetected", "allele.Change/"+changeID, conflict.GetId(),
		&allelev1.ConflictDetectedEvent{
			RepositoryId: repositoryID,
			ChangeId:     changeID,
			Conflict:     conflict,
			At:           timestamppb.Now(),
		}))
}

func (p *catalystPublisher) VerificationCompleted(repositoryID string, run *allelev1.VerificationRun) {
	p.publish(buildEvent("tooling.allele.v1.VerificationCompleted", "allele.Change/"+run.GetChangeId(), run.GetId(),
		&allelev1.VerificationCompletedEvent{
			RepositoryId: repositoryID,
			Run:          run,
			At:           timestamppb.Now(),
		}))
}

// publish sends event on the shared stream, best-effort -- same convention as lineman/crud-event's
// own publish: a failure to build or send an event is logged, never surfaced as an RPC error. This
// is an observability side-channel; Postgres (not this stream) remains the source of truth for every
// row it describes.
func (p *catalystPublisher) publish(event *acv1.CloudEvent, err error) {
	if err != nil {
		p.logger.WithError(err).Error("failed to build catalyst event")
		return
	}
	// A Connect bidi stream's Send is not safe for concurrent use.
	p.mu.Lock()
	defer p.mu.Unlock()
	if sendErr := p.stream.Send(&acv1.ProduceRequest{Message: event}); sendErr != nil {
		p.logger.WithError(sendErr).Warn("failed to publish catalyst event")
	}
}

// buildEvent matches the CloudEvent envelope convention documented in
// docs/architecture/wide-events.md's "CloudEvent envelope" table -- id = a fresh uuid, source = the
// caller's own dynamic prefix+id string, type = package + message name, data = protojson-marshaled
// payload, subject = the inner entity's own id.
func buildEvent(eventType, source, subjectID string, payload proto.Message) (*acv1.CloudEvent, error) {
	body, err := protojson.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %T: %w", payload, err)
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      source,
		SpecVersion: "1.0",
		Type:        eventType,
		Data:        &acv1.CloudEvent_TextData{TextData: string(body)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{CeTimestamp: timestamppb.Now()}},
			"subject": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{CeString: subjectID}},
		},
	}, nil
}

// h2cClient returns an HTTP client that speaks HTTP/2 cleartext (h2c), required for gRPC streaming
// against Catalyst's plain-TCP server -- copied verbatim from
// services/examples/crud-event/service/events.go's own helper of the same name/shape (itself copied
// from services/examples/producer/main.go, per that file's own comment).
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
