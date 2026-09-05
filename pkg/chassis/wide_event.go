package chassis

// wide_event.go implements chassis's automatic per-span WideEvent
// production — see
// docs/website/content/docs/architecture/wide-events.md and its
// implementation plan's Phase 5. Both span-completion points
// (otelInterceptor.reportSpan in otel_trace.go, for NewTraceInterceptor-
// wrapped RPC handlers, and Span.End, for the manual StartSpan path) call
// produceWideEventForSpan when they finish, passing the same per-span log
// buffer OTelLogger.emit appended to (see otel_trace.go's
// spanContext/spanLogBuffer and otel_logger.go's WithContext/emit).
//
// Coupled to the same telemetry.enabled gate as tracing itself in this
// phase (see reportSpan's and Span.End's own doc comments) plus its own
// telemetry.wide_events.enabled flag — WideEvent production is off by
// default even when tracing is on, since it's a materially bigger volume
// and a new capability, not a drop-in extension of something already
// running.

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	wideeventsv1 "github.com/steady-bytes/draft/api/core/observability/wide_events/v1"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// wideEventType matches services/core/beacon/ingest/wide_events.go's own
// constant — duplicated rather than shared across a chassis<->beacon import
// for one string; keep the two in sync if this ever changes.
const wideEventType = "core.observability.wide_events.v1.WideEvent"

// wideEventProducer is a process-wide singleton, matching manualExporter's
// (otel_trace.go) lazy-on-first-use shape: opening a Produce stream against
// Catalyst eagerly at every service's startup regardless of whether it ever
// emits a WideEvent would be presumptuous, since not every chassis-based
// process cares about this.
var (
	wideEventOnce    sync.Once
	wideEventStream  *connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse]
	wideEventSendMu  sync.Mutex // guards Send -- a Connect bidi stream's Send isn't safe for concurrent use
	wideEventEnabled bool
)

func wideEventProducer() (*connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse], bool) {
	wideEventOnce.Do(func() {
		cfg := GetConfig()
		wideEventEnabled = cfg.GetBool("telemetry.wide_events.enabled")
		if !wideEventEnabled {
			return
		}
		catalystAddr := cfg.GetString("catalyst.address")
		if catalystAddr == "" {
			catalystAddr = "http://localhost:2220"
		}
		client := acConnect.NewProducerClient(wideEventH2CClient(), catalystAddr, connect.WithGRPC())
		wideEventStream = client.Produce(context.Background())
	})
	return wideEventStream, wideEventEnabled
}

// produceWideEventForSpan assembles a WideEvent from one completed span
// (traceID/spanID/parentID as raw OTel bytes, matching encodeSpan's own
// parameter shape) and produces it as a CloudEvent via Catalyst. A no-op
// when WideEvent production isn't enabled (see wideEventProducer) — this is
// a fire-and-forget observability side-channel, not
// catalyst-produce's "publishing is the whole deliverable" case, so a
// failure here must never propagate to the caller's actual request.
func produceWideEventForSpan(serviceName string, traceID, spanID, parentID []byte, name string, start, end time.Time, statusCode string, logBuf *spanLogBuffer, attrs, businessAttrs, runtimeAttrs map[string]string) {
	stream, enabled := wideEventProducer()
	if !enabled {
		return
	}

	var logs []*wideeventsv1.LogLine
	var overflow uint64
	if logBuf != nil {
		lines, o := logBuf.drain()
		overflow = o
		logs = make([]*wideeventsv1.LogLine, len(lines))
		for i, l := range lines {
			logs[i] = &wideeventsv1.LogLine{
				Timestamp: timestamppb.New(l.timestamp),
				Severity:  l.severity,
				Body:      l.body,
			}
		}
	}
	if overflow > 0 {
		if runtimeAttrs == nil {
			runtimeAttrs = make(map[string]string)
		}
		runtimeAttrs["wide_event.log_overflow"] = strconv.FormatUint(overflow, 10)
	}

	event := &wideeventsv1.WideEvent{
		TraceId:            hex.EncodeToString(traceID),
		SpanId:             hex.EncodeToString(spanID),
		ParentSpanId:       hex.EncodeToString(parentID),
		ServiceName:        serviceName,
		SpanName:           name,
		StartTime:          timestamppb.New(start),
		DurationNs:         uint64(end.Sub(start).Nanoseconds()),
		StatusCode:         statusCode,
		Logs:               logs,
		Attributes:         attrs,
		BusinessAttributes: businessAttrs,
		RuntimeAttributes:  runtimeAttrs,
	}

	body, err := protojson.Marshal(event)
	if err != nil {
		return
	}
	cloudEvent := &acv1.CloudEvent{
		Id:          event.GetSpanId(),
		Source:      "/services/" + serviceName,
		SpecVersion: "1.0",
		Type:        wideEventType,
		Data:        &acv1.CloudEvent_TextData{TextData: string(body)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{CeTimestamp: event.GetStartTime()}},
		},
	}

	wideEventSendMu.Lock()
	defer wideEventSendMu.Unlock()
	// Fire-and-forget: never fail the caller's request over a dropped
	// WideEvent, but still surface it -- matching this file's own
	// "chassis: otel exporter: ..." convention (otel_logger.go) for a
	// telemetry path that silently drops on failure rather than
	// propagating it.
	if err := stream.Send(&acv1.ProduceRequest{Message: cloudEvent}); err != nil {
		fmt.Fprintf(os.Stderr, "chassis: wide_event: failed to produce (dropping): %v\n", err)
	}
}

// wideEventH2CClient returns an HTTP client that speaks HTTP/2 cleartext
// (h2c), which is required for gRPC streaming against Catalyst's plain-TCP
// server — same duplicated-per-file helper every other Producer/Consumer
// client in this repo already has.
func wideEventH2CClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}
