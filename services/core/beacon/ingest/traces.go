package ingest

import (
	"context"
	"encoding/hex"
	"sync/atomic"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"
)

// TraceReceiver implements opentelemetry.proto.collector.trace.v1.TraceService/Export
// directly against go.opentelemetry.io/proto/otlp's generated message/gRPC
// types — no collector/receiver framework, mirroring Receiver (logs.go). There
// is no live-tail publisher counterpart here: unlike StreamLogs, Beacon has no
// StreamTraces RPC (see the design doc's Phase 5 plan — SearchTraces/GetTrace
// are both bounded, non-streaming), so TraceReceiver only needs a batch writer.
type TraceReceiver struct {
	collectortracev1.UnimplementedTraceServiceServer

	writer *SpanWriter
	logger chassis.Logger
}

func NewTraceReceiver(writer *SpanWriter, logger chassis.Logger) *TraceReceiver {
	return &TraceReceiver{writer: writer, logger: logger}
}

func (r *TraceReceiver) Export(_ context.Context, req *collectortracev1.ExportTraceServiceRequest) (*collectortracev1.ExportTraceServiceResponse, error) {
	var accepted int

	for _, rs := range req.GetResourceSpans() {
		resourceAttrs := attrsToMap(rs.GetResource().GetAttributes())
		serviceName := resourceAttrs[serviceNameAttrKey]

		for _, ss := range rs.GetScopeSpans() {
			for _, sp := range ss.GetSpans() {
				row := spanToRow(sp, serviceName)
				r.writer.Save(row)
				accepted++
			}
		}
	}

	r.logger.WithField("accepted", accepted).Trace("accepted OTLP trace export")
	return &collectortracev1.ExportTraceServiceResponse{}, nil
}

func spanToRow(sp *tracev1.Span, serviceName string) store.SpanRow {
	start := sp.GetStartTimeUnixNano()
	end := sp.GetEndTimeUnixNano()

	var durationNs uint64
	if end > start {
		durationNs = end - start
	}

	return store.SpanRow{
		TraceID:      hex.EncodeToString(sp.GetTraceId()),
		SpanID:       hex.EncodeToString(sp.GetSpanId()),
		ParentSpanID: hex.EncodeToString(sp.GetParentSpanId()),
		ServiceName:  serviceName,
		SpanName:     sp.GetName(),
		Kind:         sp.GetKind().String(),
		StartTime:    time.Unix(0, int64(start)).UTC(),
		DurationNs:   durationNs,
		StatusCode:   sp.GetStatus().GetCode().String(),
		Attributes:   attrsToMap(sp.GetAttributes()),
	}
}

// ─── SpanWriter ─────────────────────────────────────────────────────────────
//
// Structurally identical to Writer (writer.go) — a bounded channel, a
// ticker-driven batch flush (batchSize/flushInterval/chanCapacity, defined
// once in writer.go and shared by both writers since they're in the same
// package), and a non-blocking Save that drops under sustained overload
// rather than back-pressuring the OTLP receiver.

// SpanWriter batches store.SpanRow values and flushes them into ClickHouse.
type SpanWriter struct {
	store        store.Storer
	logger       chassis.Logger
	ch           chan store.SpanRow
	droppedTotal atomic.Uint64
}

func NewSpanWriter(s store.Storer, logger chassis.Logger) *SpanWriter {
	w := &SpanWriter{
		store:  s,
		logger: logger,
		ch:     make(chan store.SpanRow, chanCapacity),
	}
	go w.flusher()
	return w
}

// Save enqueues row for batch insertion. It never blocks the caller — if the
// channel is full the row is dropped rather than stalling the OTLP receiver.
func (w *SpanWriter) Save(row store.SpanRow) {
	select {
	case w.ch <- row:
	default:
		w.droppedTotal.Add(1)
	}
}

// DroppedTotal returns the number of rows dropped so far because the ingest
// channel was full.
func (w *SpanWriter) DroppedTotal() uint64 {
	return w.droppedTotal.Load()
}

func (w *SpanWriter) flusher() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	buf := make([]store.SpanRow, 0, batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := w.store.InsertSpans(context.Background(), buf); err != nil {
			w.logger.WithField("error", err.Error()).Error("failed to flush span batch to clickhouse")
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
