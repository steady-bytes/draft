// This file adds a second ingestion transport to the ingest package,
// alongside the OTLP receivers in logs.go/traces.go/metrics.go: WideEvents
// arrive as CloudEvents over Catalyst's Consumer.Consume stream, not OTLP.
// The package's charter ("gets an external signal batched into ClickHouse")
// still fits — this is the same Writer shape (see writer.go) under a
// different transport, not a different package.
package ingest

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	wideeventsv1 "github.com/steady-bytes/draft/api/core/observability/wide_events/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
)

// wideEventType is the CloudEvent type every WideEvent producer (chassis's
// automatic per-span production, or Beacon's own CreateWideEvent RPC) uses —
// see docs/website/content/docs/architecture/wide-events.md's CloudEvent
// envelope table.
const wideEventType = "core.observability.wide_events.v1.WideEvent"

// reconnectDelay is how long ConsumeWideEvents waits before retrying a
// failed or ended Consume stream. Unlike examples/consumer's fire-once demo
// process, Beacon must keep ingesting across a Catalyst restart until Beacon
// itself is also restarted.
const reconnectDelay = 2 * time.Second

// WideEventWriter batches store.WideEventRow values and flushes them into
// ClickHouse. Same shape as Writer (writer.go) — a bounded channel, a
// ticker-driven batch flush, and a non-blocking Save that drops under
// sustained overload rather than back-pressuring the consume loop.
type WideEventWriter struct {
	store        store.Storer
	logger       chassis.Logger
	ch           chan store.WideEventRow
	droppedTotal atomic.Uint64
}

func NewWideEventWriter(s store.Storer, logger chassis.Logger) *WideEventWriter {
	w := &WideEventWriter{
		store:  s,
		logger: logger,
		ch:     make(chan store.WideEventRow, chanCapacity),
	}
	go w.flusher()
	return w
}

// Save enqueues row for batch insertion. It never blocks the caller — if the
// channel is full the row is dropped rather than stalling the consume loop.
func (w *WideEventWriter) Save(row store.WideEventRow) {
	select {
	case w.ch <- row:
	default:
		w.droppedTotal.Add(1)
	}
}

// DroppedTotal returns the number of rows dropped so far because the ingest
// channel was full.
func (w *WideEventWriter) DroppedTotal() uint64 {
	return w.droppedTotal.Load()
}

func (w *WideEventWriter) flusher() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	buf := make([]store.WideEventRow, 0, batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := w.store.InsertWideEvents(context.Background(), buf); err != nil {
			w.logger.WithField("error", err.Error()).Error("failed to flush wide_event batch to clickhouse")
		}
		buf = buf[:0]
	}

	for {
		select {
		case row := <-w.ch:
			buf = append(buf, row)
			if len(buf) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// ConsumeWideEvents opens a long-lived Consumer.Consume stream against
// Catalyst, filtered to wideEventType, and saves every received WideEvent
// via writer. Reconnects (after reconnectDelay) on any stream error rather
// than returning, for as long as ctx is not done — intended to run for the
// life of the process via chassis.WithRunner, same as
// examples/consumer/main.go's consumeType but with a retry loop that demo
// doesn't need.
func ConsumeWideEvents(ctx context.Context, logger chassis.Logger, catalystAddr string, writer *WideEventWriter) {
	client := acConnect.NewConsumerClient(h2cClient(), catalystAddr, connect.WithGRPC())

	for {
		if ctx.Err() != nil {
			return
		}

		req := connect.NewRequest(&acv1.ConsumeRequest{
			Message: &acv1.CloudEvent{Source: "/services/core-beacon", Type: wideEventType},
		})

		stream, err := client.Consume(ctx, req)
		if err != nil {
			logger.WithField("error", err.Error()).Error("wide_events: failed to open consume stream, retrying")
			time.Sleep(reconnectDelay)
			continue
		}

		logger.Info("wide_events: consume stream open")

		for stream.Receive() {
			event := stream.Msg().GetMessage()
			if event == nil || event.Type != wideEventType {
				continue
			}
			var payload wideeventsv1.WideEvent
			if err := protojson.Unmarshal([]byte(event.GetTextData()), &payload); err != nil {
				logger.WithField("error", err.Error()).Error("wide_events: failed to unmarshal")
				continue
			}
			writer.Save(wideEventProtoToRow(&payload))
		}

		if err := stream.Err(); err != nil && ctx.Err() == nil {
			logger.WithField("error", err.Error()).Error("wide_events: consume stream ended, reconnecting")
			time.Sleep(reconnectDelay)
		}
	}
}

func wideEventProtoToRow(e *wideeventsv1.WideEvent) store.WideEventRow {
	logs := make([]store.WideEventLogLine, len(e.GetLogs()))
	for i, l := range e.GetLogs() {
		logs[i] = store.WideEventLogLine{
			Timestamp: l.GetTimestamp().AsTime(),
			Severity:  l.GetSeverity(),
			Body:      l.GetBody(),
		}
	}
	return store.WideEventRow{
		TraceID:            e.GetTraceId(),
		SpanID:             e.GetSpanId(),
		ParentSpanID:       e.GetParentSpanId(),
		ServiceName:        e.GetServiceName(),
		SpanName:           e.GetSpanName(),
		StartTime:          e.GetStartTime().AsTime(),
		DurationNs:         e.GetDurationNs(),
		StatusCode:         e.GetStatusCode(),
		Logs:               logs,
		Attributes:         e.GetAttributes(),
		BusinessAttributes: e.GetBusinessAttributes(),
		RuntimeAttributes:  e.GetRuntimeAttributes(),
	}
}

// h2cClient returns an HTTP client that speaks HTTP/2 cleartext (h2c), which
// is required for gRPC streaming against Catalyst's plain-TCP server — same
// duplicated-per-file helper every other Producer/Consumer client in this
// repo already has (catalyst-produce, catalyst-consume, examples/consumer,
// etc.); small enough not to be worth extracting into pkg/chassis yet.
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
