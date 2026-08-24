package chassis

// otel_metrics.go is Phase 10 of Beacon's observability plan (see
// docs/website/content/docs/architecture/beacon-observability.md, "Phase 10 —
// chassis integration"): a background reporter that samples Go runtime
// metrics (goroutine count, GC pause, heap) on an interval and ships them to
// Beacon over OTLP as gauges.
//
// A service opts in by explicitly constructing and starting one — this is
// never wired into any existing service's main.go by this phase (see the
// Phase 10 report's scope boundary):
//
//	reporter := chassis.NewMetricsReporter()
//	reporter.Start() // non-blocking; spawns its own background goroutine
//
// See otel_logger.go's top doc comment for why the OTLP transport and
// protobuf wire encoding here are hand-rolled (no
// go.opentelemetry.io/proto/otlp, no google.golang.org/grpc ClientConn)
// rather than using generated client types.

import (
	"math"
	"runtime"
	"sync"
	"time"
)

// ─── Metrics (metrics/v1, collector/metrics/v1) ─────────────────────────────
//
// Field numbers verified against metrics.pb.go / metrics_service.pb.go in
// $GOMODCACHE/go.opentelemetry.io/proto/otlp@v1.7.0/metrics/v1/ and
// collector/metrics/v1/ — see otel_logger.go's top doc comment for the
// verification methodology shared by every OTLP message this exporter
// builds. Only the Gauge data-point shape is needed here (Go runtime
// snapshots like goroutine count and heap size are naturally
// point-in-time gauges, not cumulative sums or histograms), matching
// services/core/beacon/ingest/metrics.go's handling of `m.GetGauge()`.

// encodeNumberDataPointDouble encodes one NumberDataPoint with its
// `as_double` oneof branch set. Field numbers (metrics.pb.go):
// time_unix_nano=3 (fixed64), as_double=4 (fixed64, oneof — always written,
// even for value == 0.0, via appendFixed64Field's "always write" semantics;
// see otel_logger.go's doc comment on that helper for why skip-on-zero would
// be wrong for a oneof field).
func encodeNumberDataPointDouble(value float64, timeUnixNano uint64) []byte {
	var buf []byte
	buf = appendFixed64Field(buf, 3, timeUnixNano)
	buf = appendFixed64Field(buf, 4, math.Float64bits(value))
	return buf
}

// encodeGaugeMetric encodes one Metric carrying a single Gauge data point.
// Field numbers (metrics.pb.go): Gauge.data_points=1 (repeated
// NumberDataPoint), Metric.name=1 (string), Metric.gauge=5 (Gauge message,
// oneof).
func encodeGaugeMetric(name string, value float64, timeUnixNano uint64) []byte {
	dp := encodeNumberDataPointDouble(value, timeUnixNano)

	var gauge []byte
	gauge = appendMessageField(gauge, 1, dp)

	var metric []byte
	metric = appendStringField(metric, 1, name)
	metric = appendMessageField(metric, 5, gauge)
	return metric
}

// encodeExportMetricsRequest wraps one or more encoded Metrics into a full
// ExportMetricsServiceRequest: Metric -> ScopeMetrics.metrics=2 ->
// ResourceMetrics{resource=1, scope_metrics=2} ->
// ExportMetricsServiceRequest.resource_metrics=1 (metrics.pb.go /
// metrics_service.pb.go).
func encodeExportMetricsRequest(serviceName string, metrics [][]byte) []byte {
	var scopeMetrics []byte
	for _, m := range metrics {
		scopeMetrics = appendMessageField(scopeMetrics, 2, m)
	}

	var resourceMetrics []byte
	resourceMetrics = appendMessageField(resourceMetrics, 1, encodeResource(serviceName))
	resourceMetrics = appendMessageField(resourceMetrics, 2, scopeMetrics)

	var req []byte
	req = appendMessageField(req, 1, resourceMetrics)
	return req
}

// ─── MetricsReporter ─────────────────────────────────────────────────────────

// defaultMetricsInterval is used when telemetry.metrics_interval is unset or
// non-positive in config.yaml.
const defaultMetricsInterval = 15 * time.Second

// runtimeMetricNames are the OTel-semantic-convention-flavored metric names
// this reporter emits (dots-to-underscores already applied, matching
// services/core/beacon/ingest/metrics.go's sanitizeLabelName convention for
// names ingested from real OTel SDKs, so these are queryable through
// QueryMetrics with no special-casing).
const (
	metricGoroutines = "process_runtime_go_goroutines"
	metricHeapAlloc  = "process_runtime_go_mem_heap_alloc_bytes"
	metricHeapSys    = "process_runtime_go_mem_heap_sys_bytes"
	metricGCPauseNs  = "process_runtime_go_gc_pause_ns"
	metricGCCount    = "process_runtime_go_gc_count"
)

// MetricsReporter periodically samples Go runtime metrics and ships them to
// Beacon over OTLP as gauges.
type MetricsReporter struct {
	exporter    *OTelExporter
	serviceName string
	interval    time.Duration

	stop     chan struct{}
	stopOnce sync.Once
}

// NewMetricsReporter constructs a MetricsReporter. It reads the same
// "telemetry" config section as OTelLogger/NewTraceInterceptor (see
// OTelExporterConfig) via chassis.GetConfig(), plus an optional
// `telemetry.metrics_interval` duration (e.g. "15s") defaulting to
// defaultMetricsInterval.
func NewMetricsReporter() *MetricsReporter {
	cfg := GetConfig()
	interval := cfg.GetDuration(telemetryConfigKey + ".metrics_interval")
	if interval <= 0 {
		interval = defaultMetricsInterval
	}
	return &MetricsReporter{
		exporter:    newOTelExporter(cfg),
		serviceName: cfg.Name(),
		interval:    interval,
		stop:        make(chan struct{}),
	}
}

// Start begins sampling in a background goroutine and returns immediately.
// A no-op if telemetry.enabled is false. Safe to call at most once per
// MetricsReporter.
func (r *MetricsReporter) Start() {
	if !r.exporter.Enabled() {
		return
	}
	go r.run()
}

// Stop ends the background sampling loop. Safe to call more than once or
// concurrently; a no-op if Start was never called or the exporter is
// disabled.
func (r *MetricsReporter) Stop() {
	r.stopOnce.Do(func() { close(r.stop) })
}

func (r *MetricsReporter) run() {
	// Emit one sample immediately rather than waiting a full interval — a
	// short-lived verification run or a service that restarts often
	// shouldn't have to wait out defaultMetricsInterval to see its first
	// point land.
	r.reportOnce()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.reportOnce()
		case <-r.stop:
			return
		}
	}
}

// reportOnce samples runtime.MemStats/NumGoroutine once and exports one
// gauge metric per value named in the runtimeMetricNames block above.
func (r *MetricsReporter) reportOnce() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	now := uint64(time.Now().UnixNano())

	var lastPauseNs uint64
	if ms.NumGC > 0 {
		// PauseNs is a ring buffer of the most recent 256 GC pauses; the
		// most recent one is at index (NumGC+255)%256 — the same indexing
		// runtime.MemStats.PauseNs's own doc comment specifies.
		lastPauseNs = ms.PauseNs[(ms.NumGC+255)%256]
	}

	metrics := [][]byte{
		encodeGaugeMetric(metricGoroutines, float64(runtime.NumGoroutine()), now),
		encodeGaugeMetric(metricHeapAlloc, float64(ms.HeapAlloc), now),
		encodeGaugeMetric(metricHeapSys, float64(ms.HeapSys), now),
		encodeGaugeMetric(metricGCPauseNs, float64(lastPauseNs), now),
		encodeGaugeMetric(metricGCCount, float64(ms.NumGC), now),
	}

	r.exporter.Export(metricsExportProcedure, encodeExportMetricsRequest(r.serviceName, metrics))
}
