package ingest

import (
	"context"
	"maps"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"

	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"
)

// MetricsReceiver implements
// opentelemetry.proto.collector.metrics.v1.MetricsService/Export directly
// against go.opentelemetry.io/proto/otlp's generated message/gRPC types — no
// collector/receiver framework, mirroring Receiver (logs.go) and
// TraceReceiver (traces.go). Like traces, there is no live-tail counterpart —
// Beacon has no StreamMetrics RPC, only the bounded QueryMetrics RPC (see the
// design doc's Phase 8 plan) — so MetricsReceiver only needs a batch writer.
type MetricsReceiver struct {
	collectormetricsv1.UnimplementedMetricsServiceServer

	writer *MetricWriter
	logger chassis.Logger
}

func NewMetricsReceiver(writer *MetricWriter, logger chassis.Logger) *MetricsReceiver {
	return &MetricsReceiver{writer: writer, logger: logger}
}

func (r *MetricsReceiver) Export(_ context.Context, req *collectormetricsv1.ExportMetricsServiceRequest) (*collectormetricsv1.ExportMetricsServiceResponse, error) {
	var accepted int

	for _, rm := range req.GetResourceMetrics() {
		resourceLabels := labelsFromAttrs(rm.GetResource().GetAttributes())

		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				rows := metricToRows(m, resourceLabels)
				for _, row := range rows {
					r.writer.Save(row)
				}
				accepted += len(rows)
			}
		}
	}

	r.logger.WithField("accepted", accepted).Trace("accepted OTLP metrics export")
	return &collectormetricsv1.ExportMetricsServiceResponse{}, nil
}

// metricToRows flattens one OTLP Metric's data points into store.MetricPointRow
// values, following the design doc's Phase 7 convention: gauge/sum data
// points become one (metric_name, labels, timestamp, value) row each;
// histogram data points are flattened into `_bucket` (one per bucket, with a
// synthetic `le` label carrying the cumulative upper bound — the Prometheus
// exposition convention), `_sum`, and `_count` rows, so the PromQL-subset
// query layer (query/promql.go) never needs to special-case histograms.
// Summary and exponential-histogram data points are not yet supported (rare
// in practice, and out of the "deliberately small" v1 scope) — they're
// skipped rather than dropping the whole export.
func metricToRows(m *metricsv1.Metric, resourceLabels map[string]string) []store.MetricPointRow {
	// Sanitized the same way as labels (sanitizeLabelName) — OTel semantic-
	// convention metric names commonly contain dots (e.g.
	// "process.runtime.go.goroutines"), which aren't valid in a PromQL-subset
	// identifier. Mirrors the standard OTel-to-Prometheus exporter convention
	// of turning dots into underscores at the ingestion boundary, so every
	// ingested metric is queryable by name with no separate translation layer
	// at query time.
	name := sanitizeLabelName(m.GetName())

	switch {
	case m.GetGauge() != nil:
		return numberDataPointsToRows(name, m.GetGauge().GetDataPoints(), resourceLabels)
	case m.GetSum() != nil:
		return numberDataPointsToRows(name, m.GetSum().GetDataPoints(), resourceLabels)
	case m.GetHistogram() != nil:
		return histogramDataPointsToRows(name, m.GetHistogram().GetDataPoints(), resourceLabels)
	default:
		// Summary / ExponentialHistogram — unsupported in this scoped v1.
		return nil
	}
}

func numberDataPointsToRows(name string, dps []*metricsv1.NumberDataPoint, resourceLabels map[string]string) []store.MetricPointRow {
	rows := make([]store.MetricPointRow, 0, len(dps))
	for _, dp := range dps {
		rows = append(rows, store.MetricPointRow{
			MetricName: name,
			Labels:     mergeLabels(resourceLabels, labelsFromAttrs(dp.GetAttributes())),
			Timestamp:  metricTimestamp(dp.GetTimeUnixNano()),
			Value:      numberDataPointValue(dp),
		})
	}
	return rows
}

func histogramDataPointsToRows(name string, dps []*metricsv1.HistogramDataPoint, resourceLabels map[string]string) []store.MetricPointRow {
	var rows []store.MetricPointRow
	for _, dp := range dps {
		ts := metricTimestamp(dp.GetTimeUnixNano())
		base := mergeLabels(resourceLabels, labelsFromAttrs(dp.GetAttributes()))

		// OTLP's bucket_counts are per-bucket (non-cumulative); Prometheus's
		// `_bucket{le=...}` convention is cumulative. explicit_bounds has one
		// fewer entry than bucket_counts — the last bucket is implicitly +Inf.
		bounds := dp.GetExplicitBounds()
		counts := dp.GetBucketCounts()
		var cumulative uint64
		for i, count := range counts {
			cumulative += count
			le := "+Inf"
			if i < len(bounds) {
				le = strconv.FormatFloat(bounds[i], 'g', -1, 64)
			}
			bucketLabels := mergeLabels(base, map[string]string{"le": le})
			rows = append(rows, store.MetricPointRow{
				MetricName: name + "_bucket",
				Labels:     bucketLabels,
				Timestamp:  ts,
				Value:      float64(cumulative),
			})
		}

		rows = append(rows,
			store.MetricPointRow{
				MetricName: name + "_sum",
				Labels:     mergeLabels(base, nil),
				Timestamp:  ts,
				Value:      dp.GetSum(),
			},
			store.MetricPointRow{
				MetricName: name + "_count",
				Labels:     mergeLabels(base, nil),
				Timestamp:  ts,
				Value:      float64(dp.GetCount()),
			},
		)
	}
	return rows
}

func numberDataPointValue(dp *metricsv1.NumberDataPoint) float64 {
	switch v := dp.GetValue().(type) {
	case *metricsv1.NumberDataPoint_AsDouble:
		return v.AsDouble
	case *metricsv1.NumberDataPoint_AsInt:
		return float64(v.AsInt)
	default:
		return 0
	}
}

func metricTimestamp(unixNano uint64) time.Time {
	return time.Unix(0, int64(unixNano)).UTC()
}

// labelSanitizer matches any character not valid in a PromQL-subset label
// identifier ([a-zA-Z_][a-zA-Z0-9_]*) — mirrors the standard OTel-to-
// Prometheus exporter convention of turning e.g. "service.name" into
// "service_name" so every ingested label is usable as a query/promql.go
// LabelMatcher name without a translation layer at query time.
var labelSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitizeLabelName(name string) string {
	if name == "" {
		return name
	}
	sanitized := labelSanitizer.ReplaceAllString(name, "_")
	if sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "_" + sanitized
	}
	return sanitized
}

// labelsFromAttrs reuses attrsToMap (logs.go) for the actual key/value
// extraction, then sanitizes each key into a valid PromQL-subset label
// identifier.
func labelsFromAttrs(kvs []*commonv1.KeyValue) map[string]string {
	raw := attrsToMap(kvs)
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		out[sanitizeLabelName(k)] = v
	}
	return out
}

// mergeLabels combines resource-level and data-point-level labels into one
// map, with dp-level labels taking precedence on key collision (mirrors how
// Prometheus resolves target/sample label conflicts) — a and b may each be
// nil.
func mergeLabels(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	maps.Copy(out, a)
	maps.Copy(out, b)
	return out
}

// ─── MetricWriter ───────────────────────────────────────────────────────────
//
// Structurally identical to Writer/SpanWriter — a bounded channel, a
// ticker-driven batch flush (batchSize/flushInterval/chanCapacity, shared
// package-level consts from writer.go), and a non-blocking Save that drops
// under sustained overload rather than back-pressuring the OTLP receiver.

// MetricWriter batches store.MetricPointRow values and flushes them into
// ClickHouse.
type MetricWriter struct {
	store        store.Storer
	logger       chassis.Logger
	ch           chan store.MetricPointRow
	droppedTotal atomic.Uint64
}

func NewMetricWriter(s store.Storer, logger chassis.Logger) *MetricWriter {
	w := &MetricWriter{
		store:  s,
		logger: logger,
		ch:     make(chan store.MetricPointRow, chanCapacity),
	}
	go w.flusher()
	return w
}

// Save enqueues row for batch insertion. It never blocks the caller — if the
// channel is full the row is dropped rather than stalling the OTLP receiver.
func (w *MetricWriter) Save(row store.MetricPointRow) {
	select {
	case w.ch <- row:
	default:
		w.droppedTotal.Add(1)
	}
}

// DroppedTotal returns the number of rows dropped so far because the ingest
// channel was full.
func (w *MetricWriter) DroppedTotal() uint64 {
	return w.droppedTotal.Load()
}

func (w *MetricWriter) flusher() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	buf := make([]store.MetricPointRow, 0, batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := w.store.InsertMetrics(context.Background(), buf); err != nil {
			w.logger.WithField("error", err.Error()).Error("failed to flush metric batch to clickhouse")
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
