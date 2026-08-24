package chassis

// otel_logger.go is Phase 10 of Beacon's observability plan (see
// docs/website/content/docs/architecture/beacon-observability.md, "Phase 10 —
// chassis integration"): a chassis.Logger implementation that ships
// structured log records to Beacon over OTLP instead of writing them to
// stdout (contrast pkg/loggers/zerolog/zerolog.go, which is stdout-only).
//
// This file also hosts the OTLP transport plumbing shared by otel_trace.go
// and otel_metrics.go — Blueprint address resolution, a bounded
// fire-and-forget send queue, and a hand-rolled gRPC-over-h2c client, plus a
// minimal protobuf wire-format encoder for the handful of OTLP v1 messages
// all three files need to build. It is not split into its own file because
// the Phase 10 scope boundary limits pkg/chassis to exactly three new files
// (otel_logger.go, otel_trace.go, otel_metrics.go) — see the task brief.
//
// ─── Why a hand-rolled protobuf encoder instead of go.opentelemetry.io/proto/otlp ───
//
// The Phase 10 brief's "decisions already in place" call for building OTLP
// Export requests against go.opentelemetry.io/proto/otlp's generated
// message/gRPC types directly (the same package Beacon's own OTLP receivers
// import — see services/core/beacon/ingest/{logs,traces,metrics}.go and
// services/core/beacon/go.mod). That package is not currently a dependency of
// pkg/chassis/go.mod, and the same brief's strict scope boundary forbids
// modifying pkg/chassis/go.mod or go.sum at all — not even a one-line
// `require` addition — because both files already carry substantial
// unrelated uncommitted changes from a different in-flight session (an
// effect-system refactor) that this phase must not disturb or entangle with.
//
// Rather than stop short of a working exporter, this file instead encodes
// the small, fixed set of OTLP v1 messages needed (KeyValue/AnyValue/Resource
// plus the log/span/metric-specific messages) directly against the
// protobuf wire format using only the standard library — no new dependency.
// The wire format itself (varint, length-delimited, fixed64 — see
// appendVarint/appendTag/append*Field below) is what proto.Marshal produces
// for any message; a decoder on the other end (including the real generated
// Go structs Beacon's receivers use, via the real proto.Unmarshal) cannot
// tell the difference between bytes produced this way and bytes produced by
// the official generated code, provided the field numbers and wire types
// match the schema. Every field number/wire type used below was read
// directly from the generated Go source of
// go.opentelemetry.io/proto/otlp@v1.7.0 — the exact version Beacon's own
// go.mod pins — in the local module cache
// ($GOMODCACHE/go.opentelemetry.io/proto/otlp@v1.7.0/{common,logs,trace,metrics}/v1/*.pb.go
// and collector/{logs,trace,metrics}/v1/*_service*.pb.go), not from memory,
// and the whole path was verified end-to-end against Beacon's real running
// OTLP receiver and QueryLogs/SearchTraces/QueryMetrics RPCs (see the Phase
// 10 report for the verification transcript).
//
// The gRPC transport itself is likewise hand-rolled directly against
// cleartext HTTP/2 (net/http + golang.org/x/net/http2, both already chassis
// dependencies) rather than pulling in google.golang.org/grpc's ClientConn —
// gRPC's unary wire framing on top of HTTP/2 (a 5-byte
// compressed-flag+length prefix per message, `content-type:
// application/grpc+proto`, `grpc-status`/`grpc-message` trailers) is a
// small, stable, fully documented part of the protocol requiring no
// generated stub to speak correctly. This mirrors, client-side, the same
// "no collector/receiver framework" hand-rolled posture Beacon's ingest
// package already takes server-side (see ingest/logs.go's doc comment).

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"
	sdv1Cnt "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1/v1connect"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
)

// ─── Config ─────────────────────────────────────────────────────────────────

// OTelExporterConfig is the "ship telemetry" config block every otel_*.go
// exporter in this package reads via Config.UnmarshalKey("telemetry", ...) —
// one UnmarshalKey call for the whole section, matching the convention
// services/core/beacon/store/clickhouse.go's ClickHouseConfig and
// services/core/catalyst/broker/store.go's ClickHouseConfig already use
// (`Enabled bool `mapstructure:"enabled"“ plus sibling settings, read in one
// shot rather than scattered individual Get* calls — see both files' `if
// chCfg.Enabled { ... } else { ...noop fallback... }` construction in their
// respective main.go). This is the closest existing precedent in this
// codebase for a plain config-driven (non-Effect-system) on/off toggle; the
// other two chassis plugins named in the brief (pkg/brokers/nats,
// pkg/repositories/postgres/{gorm,bun}) don't have an Enabled field at all —
// they're gated by whether a service calls .WithBroker()/.WithRepository()
// (the Effect system) in the first place, which Phase 10 is explicitly
// forbidden from touching (a service opts in here by explicitly constructing
// and passing an *OTelLogger/*MetricsReporter/interceptor, never through
// chassis's plugin/Effect registration).
//
// Example config.yaml section:
//
//	telemetry:
//	  enabled: true
//	  level: debug
//	  # otlp_port: 4317   # optional; defaults to Beacon's own convention
type OTelExporterConfig struct {
	// Enabled is the boolean-ish "ship telemetry: on" toggle. When false, no
	// exporter goroutine is started and every Export call is a no-op.
	Enabled bool `mapstructure:"enabled"`
	// Level is the log level threshold for OTelLogger specifically (which
	// severities actually get shipped) — this is a separate knob from
	// service.logging.level so a service can run its own console/other
	// logger at one verbosity while shipping a different verbosity to
	// Beacon. Empty means "use service.logging.level".
	Level string `mapstructure:"level"`
	// OtlpPort overrides the port appended to Beacon's Blueprint-registered
	// host to reach its OTLP receiver. Beacon's OTLP listener
	// (services/core/beacon/main.go) is a separate raw grpc.Server, not
	// registered as chassis Connect-RPC handler metadata with Blueprint (see
	// pkg/chassis/builder.go's synchronize — only rpcServiceNames populated
	// via AddHandler's `pattern` argument are advertised there), so only
	// Beacon's *host* is actually discoverable through Blueprint; the OTLP
	// port itself is a cluster-wide convention (Beacon's own config.yaml
	// defaults `otlp.bind_address` to "0.0.0.0:4317" — see
	// services/core/beacon/config.yaml) overridable here for a cluster that
	// changed it. 0 means "use the default".
	OtlpPort int `mapstructure:"otlp_port"`
}

// telemetryConfigKey is the config.yaml section OTelExporterConfig is read
// from.
const telemetryConfigKey = "telemetry"

// defaultBeaconOTLPPort mirrors services/core/beacon/config.yaml's
// `otlp.bind_address: "0.0.0.0:4317"` default.
const defaultBeaconOTLPPort = 4317

// beaconProcessName is the Blueprint-registered process name Beacon's own
// Phase 0 main.go/config.yaml uses (`service.name: beacon`, registered with
// `Namespace: "core"` — see services/core/beacon/main.go's
// chassis.New(logger).Register(chassis.RegistrationOptions{Namespace:
// "core"}) and config.yaml's `service.name: beacon`).
const beaconProcessName = "beacon"

// ─── OTLP procedure paths ───────────────────────────────────────────────────
//
// These are the fully-qualified gRPC method paths for OTLP's standard
// collector services, verified against
// $GOMODCACHE/go.opentelemetry.io/proto/otlp@v1.7.0/collector/{logs,trace,metrics}/v1/*_grpc.pb.go's
// `FullMethod`/`Invoke` constants — the same package/version Beacon's own
// go.mod requires and its ingest/{logs,traces,metrics}.go implement the
// server side of.
const (
	logsExportProcedure    = "/opentelemetry.proto.collector.logs.v1.LogsService/Export"
	tracesExportProcedure  = "/opentelemetry.proto.collector.trace.v1.TraceService/Export"
	metricsExportProcedure = "/opentelemetry.proto.collector.metrics.v1.MetricsService/Export"
)

// ─── Protobuf wire encoding helpers ─────────────────────────────────────────
//
// Minimal varint/length-delimited/fixed64 protobuf wire-format writer. See
// this file's top doc comment for why this exists instead of importing
// go.opentelemetry.io/proto/otlp. Field numbers/wire types used by callers
// below are cited at each call site.

// appendVarint appends v as a protobuf base-128 varint.
func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

// appendTag appends a field tag: (fieldNum << 3) | wireType, as a varint.
// wireType 0 = varint, 1 = 64-bit (fixed64/double), 2 = length-delimited
// (string/bytes/embedded message).
func appendTag(buf []byte, fieldNum, wireType int) []byte {
	return appendVarint(buf, uint64(fieldNum)<<3|uint64(wireType))
}

// appendVarintField always writes fieldNum/v — used for enum and other
// varint scalar fields where "unset" and "explicit zero" don't need to be
// distinguished for our purposes.
func appendVarintField(buf []byte, fieldNum int, v uint64) []byte {
	buf = appendTag(buf, fieldNum, 0)
	return appendVarint(buf, v)
}

// appendFixed64Field always writes fieldNum/v as 8 little-endian bytes —
// used for fixed64 timestamp fields and, via math.Float64bits, oneof
// `double` fields (see otel_metrics.go's encodeNumberDataPointDouble) where
// skipping an explicit zero value would silently misrepresent a legitimate
// zero-valued gauge as "unset".
func appendFixed64Field(buf []byte, fieldNum int, v uint64) []byte {
	buf = appendTag(buf, fieldNum, 1)
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], v)
	return append(buf, tmp[:]...)
}

// appendStringField omits the field entirely when s is empty — proto3's
// standard zero-value-omission convention, safe here because none of this
// file's string fields are oneofs where "empty" and "unset" must be
// distinguished.
func appendStringField(buf []byte, fieldNum int, s string) []byte {
	if s == "" {
		return buf
	}
	buf = appendTag(buf, fieldNum, 2)
	buf = appendVarint(buf, uint64(len(s)))
	return append(buf, s...)
}

// appendBytesField omits the field entirely when b is empty (e.g. an unset
// trace_id/parent_span_id).
func appendBytesField(buf []byte, fieldNum int, b []byte) []byte {
	if len(b) == 0 {
		return buf
	}
	buf = appendTag(buf, fieldNum, 2)
	buf = appendVarint(buf, uint64(len(b)))
	return append(buf, b...)
}

// appendMessageField wraps an already-encoded embedded message's bytes as a
// length-delimited field. Omits the field entirely when msg is empty, which
// is never semantically meaningful for any embedded message this file
// constructs (Resource, AnyValue, Status, etc. are never intentionally
// "present but empty").
func appendMessageField(buf []byte, fieldNum int, msg []byte) []byte {
	if len(msg) == 0 {
		return buf
	}
	buf = appendTag(buf, fieldNum, 2)
	buf = appendVarint(buf, uint64(len(msg)))
	return append(buf, msg...)
}

// ─── Common OTLP messages (common/v1, resource/v1) ─────────────────────────
//
// Field numbers verified against common.pb.go/resource.pb.go in
// $GOMODCACHE/go.opentelemetry.io/proto/otlp@v1.7.0/{common,resource}/v1/.

// encodeAnyValueString encodes an AnyValue with its `string_value` oneof
// branch set (common.pb.go: `StringValue string
// protobuf:"bytes,1,opt,name=string_value,...,oneof"`). Every attribute value
// and log body this exporter ever sends is a string — chassis.Fields values
// are stringified with fmt.Sprintf("%v", ...) before reaching here, mirroring
// pkg/loggers/zerolog/zerolog.go's WithFields, which does the same.
func encodeAnyValueString(s string) []byte {
	var buf []byte
	return appendStringField(buf, 1, s)
}

// encodeKeyValue encodes a KeyValue (common.pb.go: `Key string
// protobuf:"bytes,1,...`, `Value *AnyValue protobuf:"bytes,2,...`).
func encodeKeyValue(key, value string) []byte {
	var buf []byte
	buf = appendStringField(buf, 1, key)
	buf = appendMessageField(buf, 2, encodeAnyValueString(value))
	return buf
}

// encodeAttributesRepeated encodes attrs as a series of repeated KeyValue
// fields at fieldNum, sorted by key for deterministic output (helps manual
// inspection/verification; wire semantics of a repeated field don't depend
// on order).
func encodeAttributesRepeated(fieldNum int, attrs map[string]string) []byte {
	if len(attrs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf []byte
	for _, k := range keys {
		buf = appendMessageField(buf, fieldNum, encodeKeyValue(k, attrs[k]))
	}
	return buf
}

// encodeResource encodes a Resource carrying just the OTel semantic
// convention `service.name` attribute (resource.pb.go: `Attributes
// []*v1.KeyValue protobuf:"bytes,1,rep,...`) — the same resource attribute
// key Beacon's own ingest/logs.go reads back out via serviceNameAttrKey =
// "service.name".
func encodeResource(serviceName string) []byte {
	if serviceName == "" {
		return nil
	}
	var buf []byte
	return appendMessageField(buf, 1, encodeKeyValue("service.name", serviceName))
}

// ─── Logs (logs/v1, collector/logs/v1) ──────────────────────────────────────
//
// Field numbers verified against logs.pb.go / logs_service.pb.go in
// $GOMODCACHE/go.opentelemetry.io/proto/otlp@v1.7.0/logs/v1/ and
// collector/logs/v1/.

// logRecordInput is the minimal set of fields otel_logger.go's OTelLogger
// needs to populate one LogRecord.
type logRecordInput struct {
	timeUnixNano   uint64
	severityNumber uint32
	severityText   string
	body           string
	attributes     map[string]string
	traceIDHex     string
	spanIDHex      string
}

// encodeLogRecord encodes one LogRecord. Field numbers (logs.pb.go):
// time_unix_nano=1 (fixed64), severity_number=2 (varint enum),
// severity_text=3 (string), body=5 (AnyValue message), attributes=6
// (repeated KeyValue), trace_id=9 (bytes), span_id=10 (bytes).
func encodeLogRecord(in logRecordInput) []byte {
	var buf []byte
	buf = appendFixed64Field(buf, 1, in.timeUnixNano)
	buf = appendVarintField(buf, 2, uint64(in.severityNumber))
	buf = appendStringField(buf, 3, in.severityText)
	if in.body != "" {
		buf = appendMessageField(buf, 5, encodeAnyValueString(in.body))
	}
	buf = append(buf, encodeAttributesRepeated(6, in.attributes)...)
	if tid, err := hex.DecodeString(in.traceIDHex); err == nil && len(tid) > 0 {
		buf = appendBytesField(buf, 9, tid)
	}
	if sid, err := hex.DecodeString(in.spanIDHex); err == nil && len(sid) > 0 {
		buf = appendBytesField(buf, 10, sid)
	}
	return buf
}

// encodeExportLogsRequest wraps one or more encoded LogRecords into a full
// ExportLogsServiceRequest: LogRecord -> ScopeLogs.log_records=2 ->
// ResourceLogs{resource=1, scope_logs=2} ->
// ExportLogsServiceRequest.resource_logs=1 (logs.pb.go / logs_service.pb.go).
func encodeExportLogsRequest(serviceName string, records [][]byte) []byte {
	var scopeLogs []byte
	for _, r := range records {
		scopeLogs = appendMessageField(scopeLogs, 2, r)
	}

	var resourceLogs []byte
	resourceLogs = appendMessageField(resourceLogs, 1, encodeResource(serviceName))
	resourceLogs = appendMessageField(resourceLogs, 2, scopeLogs)

	var req []byte
	req = appendMessageField(req, 1, resourceLogs)
	return req
}

// ─── OTLP severity numbers ───────────────────────────────────────────────────
//
// Verified against logs.pb.go's SeverityNumber_SEVERITY_NUMBER_* consts.
const (
	otlpSeverityTrace = 1
	otlpSeverityDebug = 5
	otlpSeverityInfo  = 9
	otlpSeverityWarn  = 13
	otlpSeverityError = 17
	otlpSeverityFatal = 21
)

// severityNumberFor maps a chassis.LogLevel to the closest OTel
// SeverityNumber. chassis has no dedicated "panic" rung in OTel's severity
// scale, so Panic is reported at the same (highest) severity as Fatal — the
// distinction chassis cares about (os.Exit vs panic) is a process-lifecycle
// concern, not a log-severity one.
func severityNumberFor(level LogLevel) uint32 {
	switch level {
	case PanicLevel, FatalLevel:
		return otlpSeverityFatal
	case ErrorLevel:
		return otlpSeverityError
	case WarnLevel:
		return otlpSeverityWarn
	case InfoLevel:
		return otlpSeverityInfo
	case DebugLevel:
		return otlpSeverityDebug
	case TraceLevel:
		return otlpSeverityTrace
	default:
		return otlpSeverityInfo
	}
}

// ─── Shared OTLP transport: Blueprint lookup + fire-and-forget send queue ──

const (
	// blueprintResolveTimeout bounds a single Blueprint Query call.
	blueprintResolveTimeout = 2 * time.Second
	// resolveCacheTTL controls how long a resolved Beacon address is reused
	// before the exporter re-queries Blueprint — short enough that Beacon
	// moving/restarting on a new address is picked up without requiring the
	// host service to restart, long enough that steady-state export traffic
	// doesn't hammer Blueprint with a Query call per message.
	resolveCacheTTL = 30 * time.Second
	// exportSendTimeout bounds a single OTLP Export call to Beacon.
	exportSendTimeout = 2 * time.Second
	// exportQueueSize bounds the fire-and-forget send queue. Once full,
	// Export drops rather than blocking the caller (see Export's doc
	// comment) — this is the "buffer briefly and drop" behavior the design
	// doc calls for.
	exportQueueSize = 256
	// warnThrottle limits how often resolution/send failures are printed to
	// stderr, so a persistently unreachable Beacon doesn't spam logs.
	warnThrottle = 10 * time.Second
)

// otlpJob is one already wire-encoded OTLP Export request body queued for
// fire-and-forget delivery.
type otlpJob struct {
	procedure string
	body      []byte
}

// OTelExporter resolves Beacon's OTLP address via Blueprint and delivers
// already wire-encoded OTLP Export request bodies to it, fire-and-forget.
// Shared internal plumbing for OTelLogger (this file), the trace interceptor
// (otel_trace.go), and the metrics reporter (otel_metrics.go) — each
// constructs and owns its own *OTelExporter (and, if enabled, its own
// background goroutine); nothing here is a process-wide singleton, matching
// each component's "opt in via explicit construction" story from the Phase
// 10 brief.
type OTelExporter struct {
	enabled  bool
	otlpPort int
	cfg      Config

	httpClient *http.Client
	ch         chan otlpJob

	resMu      sync.Mutex
	addr       string
	resolvedAt time.Time
	lastWarnAt time.Time
}

// newOTelExporter reads the "telemetry" config section (see
// OTelExporterConfig) from cfg and, if enabled, starts the background send
// loop. When disabled, Export is a permanent no-op and no goroutine is
// started — zero steady-state cost for a service that doesn't opt in.
func newOTelExporter(cfg Config) *OTelExporter {
	var ecfg OTelExporterConfig
	if err := cfg.UnmarshalKey(telemetryConfigKey, &ecfg); err != nil {
		fmt.Fprintf(os.Stderr, "chassis: otel exporter: failed to read %q config section (telemetry export disabled): %v\n", telemetryConfigKey, err)
	}

	port := ecfg.OtlpPort
	if port == 0 {
		port = defaultBeaconOTLPPort
	}

	e := &OTelExporter{
		enabled:  ecfg.Enabled,
		otlpPort: port,
		cfg:      cfg,
		httpClient: &http.Client{
			// h2c client construction mirrors pkg/chassis/builder.go's
			// newBlueprintClient and tools/blueprint-client/main.go's
			// synchronize — the same "AllowHTTP + DialTLS returns a plain
			// TCP conn" prior-knowledge-h2c trick already used throughout
			// this codebase to reach chassis services without TLS. Beacon's
			// OTLP listener is a plain grpc.Server on a raw net.Listener
			// (services/core/beacon/main.go), which — like grpc-go's own
			// insecure client — expects the HTTP/2 connection preface
			// immediately with no HTTP/1.1 upgrade, exactly what this
			// transport produces.
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
					return net.DialTimeout(network, addr, blueprintResolveTimeout)
				},
			},
		},
		ch: make(chan otlpJob, exportQueueSize),
	}

	if e.enabled {
		go e.run()
	}

	return e
}

// Enabled reports whether this exporter was configured on.
func (e *OTelExporter) Enabled() bool {
	return e != nil && e.enabled
}

// Export enqueues an already wire-encoded OTLP Export request body for
// fire-and-forget delivery to Beacon's `procedure` unary RPC. Never blocks
// the caller: a disabled exporter or a full queue both simply drop the
// message, consistent with the design doc's "the exporter buffers briefly
// and drops rather than blocking the service's own request path."
func (e *OTelExporter) Export(procedure string, body []byte) {
	if !e.Enabled() {
		return
	}
	select {
	case e.ch <- otlpJob{procedure: procedure, body: body}:
	default:
		// queue full — drop.
	}
}

// ExportSync is Export's synchronous, best-effort counterpart used only by
// OTelLogger.Fatal/Panic (otel_logger.go), where the process is about to
// exit and an async queue entry could otherwise be lost entirely. It bounds
// its own wait to exportSendTimeout and never panics on failure — a failed
// or unreachable Beacon must never prevent the fatal/panic path itself from
// proceeding.
func (e *OTelExporter) ExportSync(procedure string, body []byte) {
	if !e.Enabled() {
		return
	}
	addr, ok := e.resolvedAddr()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), exportSendTimeout)
	defer cancel()
	_ = doUnaryGRPC(ctx, e.httpClient, addr, procedure, body)
}

// run drains the send queue in the background for the lifetime of the
// process (there is no Stop — OTelExporter's goroutine exits naturally when
// the process does, mirroring how chassis.Runtime itself has no explicit
// per-plugin shutdown hook for this kind of best-effort background sender;
// contrast otel_metrics.go's MetricsReporter, which does have an explicit
// Stop since its ticker would otherwise leak in a long-running test/harness
// process).
func (e *OTelExporter) run() {
	for job := range e.ch {
		addr, ok := e.resolvedAddr()
		if !ok {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), exportSendTimeout)
		err := doUnaryGRPC(ctx, e.httpClient, addr, job.procedure, job.body)
		cancel()
		if err != nil {
			e.warnThrottled(fmt.Sprintf("chassis: otel exporter: export to beacon failed (dropping): %v\n", err))
		}
	}
}

// resolvedAddr returns Beacon's OTLP "host:port", resolving (or
// re-resolving, past resolveCacheTTL) via Blueprint's ServiceDiscoveryService
// as needed. The second return value is false when no running "beacon"
// process is currently registered with Blueprint or Blueprint itself
// couldn't be reached — callers must treat that as "drop this message,"
// never as a reason to block or retry inline.
func (e *OTelExporter) resolvedAddr() (string, bool) {
	e.resMu.Lock()
	defer e.resMu.Unlock()

	if e.addr != "" && time.Since(e.resolvedAt) < resolveCacheTTL {
		return e.addr, true
	}

	ctx, cancel := context.WithTimeout(context.Background(), blueprintResolveTimeout)
	defer cancel()
	addr, err := lookupBeaconOTLPAddr(ctx, e.cfg, e.otlpPort, e.httpClient)
	e.resolvedAt = time.Now()
	if err != nil {
		e.addr = ""
		e.warnThrottledLocked(fmt.Sprintf("chassis: otel exporter: failed to resolve beacon via blueprint (will retry, dropping telemetry meanwhile): %v\n", err))
		return "", false
	}
	e.addr = addr
	return addr, true
}

// warnThrottled acquires resMu before delegating to warnThrottledLocked —
// used by callers (run) that don't already hold it.
func (e *OTelExporter) warnThrottled(msg string) {
	e.resMu.Lock()
	defer e.resMu.Unlock()
	e.warnThrottledLocked(msg)
}

// warnThrottledLocked prints msg to stderr at most once per warnThrottle
// window. Must be called with resMu held.
func (e *OTelExporter) warnThrottledLocked(msg string) {
	if time.Since(e.lastWarnAt) < warnThrottle {
		return
	}
	e.lastWarnAt = time.Now()
	fmt.Fprint(os.Stderr, msg)
}

// ─── Blueprint service-discovery lookup ─────────────────────────────────────

// lookupBeaconOTLPAddr resolves Beacon's OTLP "host:port" via Blueprint's
// ServiceDiscoveryService.Query — the same lookup every other Draft service
// already uses to find its peers. This mirrors, field for field:
//
//   - pkg/chassis/builder.go's newBlueprintClient for the h2c client
//     construction (AllowHTTP + DialTLS returning a plain net.Dial) and
//     sdv1Cnt.NewServiceDiscoveryServiceClient(httpClient, entrypoint)
//     construction.
//   - services/tooling/bench/discovery.go's blueprintResolver.load /
//     ResolveByProcessName for the "query everything once (QueryRequest.Filter
//     is ignored by Blueprint's current implementation — see that file's doc
//     comment), then match client-side by Process.Name + PROCESS_RUNNING
//     state, using Process.IpAddress (despite the name, a full "host:port")"
//     resolution pattern — the closest existing precedent in this codebase
//     for "resolve a process's own service.name (not an RPC service it
//     serves) to an address," which is exactly what's needed here: Beacon's
//     OTLP receiver is a bare grpc.Server never registered via chassis's
//     AddHandler (see services/core/beacon/main.go), so it never appears in
//     any process's advertised RPC-service Metadata — only Process.Name
//     ("beacon", from its config.yaml's service.name) identifies it.
//
// The discovered host is combined with otlpPort (Beacon's OTLP receiver is a
// separate listener from its Blueprint-advertised Connect-RPC address — see
// OTelExporterConfig.OtlpPort's doc comment for why only the host, not the
// full OTLP address, is actually discoverable this way).
func lookupBeaconOTLPAddr(ctx context.Context, cfg Config, otlpPort int, httpClient *http.Client) (string, error) {
	entrypoint := cfg.GetString("service.entrypoint")
	if entrypoint == "" {
		return "", fmt.Errorf("service.entrypoint not configured; cannot reach blueprint")
	}

	client := sdv1Cnt.NewServiceDiscoveryServiceClient(httpClient, entrypoint)
	res, err := client.Query(ctx, connect.NewRequest(&sdv1.QueryRequest{}))
	if err != nil {
		return "", fmt.Errorf("query blueprint service discovery: %w", err)
	}

	for _, process := range res.Msg.GetData() {
		if process.GetName() != beaconProcessName {
			continue
		}
		if process.GetRunningState() != sdv1.ProcessRunningState_PROCESS_RUNNING {
			continue
		}
		host, _, err := net.SplitHostPort(process.GetIpAddress())
		if err != nil || host == "" {
			continue
		}
		return fmt.Sprintf("%s:%d", host, otlpPort), nil
	}

	return "", fmt.Errorf("no running %q process registered with blueprint", beaconProcessName)
}

// ─── Hand-rolled unary gRPC-over-h2c client ─────────────────────────────────

// doUnaryGRPC performs one unary gRPC call over cleartext HTTP/2, sending an
// already wire-encoded protobuf message body and discarding the (unparsed)
// response body — every OTLP collector Export RPC's success response is
// either empty or an optional partial-success message this exporter has no
// use for; only the grpc-status matters. See this file's top doc comment for
// why no grpc-go/generated-stub dependency is used here.
func doUnaryGRPC(ctx context.Context, client *http.Client, addr, procedure string, reqBody []byte) error {
	frame := make([]byte, 5+len(reqBody))
	// byte 0: compressed flag (0 = not compressed).
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(reqBody)))
	copy(frame[5:], reqBody)

	url := "http://" + addr + procedure
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(frame))
	if err != nil {
		return fmt.Errorf("build otlp export request: %w", err)
	}
	req.Header.Set("content-type", "application/grpc+proto")
	req.Header.Set("te", "trailers")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send otlp export request: %w", err)
	}
	defer resp.Body.Close()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("read otlp export response: %w", err)
	}

	if status := grpcStatus(resp); status != "" && status != "0" {
		return fmt.Errorf("otlp export failed: grpc-status=%s grpc-message=%s", status, grpcMessage(resp))
	}
	return nil
}

// grpcStatus reads the grpc-status value, preferring the HTTP trailer (the
// normal location for a unary RPC that got as far as a response) and falling
// back to the header (used when a server rejects a call before any response
// body, e.g. an unimplemented method).
func grpcStatus(resp *http.Response) string {
	if s := resp.Trailer.Get("grpc-status"); s != "" {
		return s
	}
	return resp.Header.Get("grpc-status")
}

func grpcMessage(resp *http.Response) string {
	if s := resp.Trailer.Get("grpc-message"); s != "" {
		return s
	}
	return resp.Header.Get("grpc-message")
}

// ─── OTelLogger ──────────────────────────────────────────────────────────────

// OTelLogger implements chassis.Logger (see logger.go) by exporting every
// log record over OTLP to Beacon instead of writing it to stdout —
// contrast pkg/loggers/zerolog/zerolog.go, which is chassis's stdout-only
// reference implementation and this file's structural template (the
// immutable-with-copy WithField/WithFields/WithCallDepth chaining pattern
// below mirrors it directly).
//
// A service opts in by constructing one and passing it to chassis.New, the
// same way services/core/catalyst/main.go does `chassis.New(zerolog.New())`:
//
//	defer chassis.New(chassis.NewOTelLogger()).
//		Register(chassis.RegistrationOptions{Namespace: "core"}).
//		WithRPCHandler(rpc).
//		Start()
//
// This is never wired into any existing service's main.go by this phase —
// see the Phase 10 report's scope boundary.
type OTelLogger struct {
	exporter    *OTelExporter
	serviceName string
	level       LogLevel
	fields      Fields
	depth       int
}

// NewOTelLogger constructs an OTelLogger. It does nothing observable until
// Start is called (by chassis.New) — Export is a no-op and GetLevel returns
// the zero LogLevel (PanicLevel) until then, consistent with logger.go's
// "Start configures the logger for service startup" contract.
func NewOTelLogger() Logger {
	return &OTelLogger{fields: make(Fields), depth: 2}
}

func (l *OTelLogger) Start(config Config) {
	l.exporter = newOTelExporter(config)
	l.serviceName = config.Name()

	l.level = ParseLogLevel(config.GetString("service.logging.level"))
	var ecfg OTelExporterConfig
	if err := config.UnmarshalKey(telemetryConfigKey, &ecfg); err == nil && ecfg.Level != "" {
		l.level = ParseLogLevel(ecfg.Level)
	}

	fmt.Fprintf(os.Stderr, "chassis: otel logger starting — service=%q beacon telemetry enabled=%v level=%s\n",
		l.serviceName, l.exporter.Enabled(), l.level)
}

func (l *OTelLogger) SetLevel(level LogLevel) {
	l.level = level
}

func (l *OTelLogger) GetLevel() LogLevel {
	return l.level
}

func (l *OTelLogger) Wrap(err error) error {
	return Wrap(err, l.fields)
}

func (l *OTelLogger) WithError(err error) Logger {
	return l.WithFields(Fields{"error": err.Error()})
}

// WithContext correlates subsequent log records with the current RPC trace,
// if any. chassis.NewTraceInterceptor() (otel_trace.go) attaches the
// request's trace_id/span_id to ctx before calling the handler; this reads
// them back out and sets them the same way an explicit
// WithField("trace_id", ...)/WithField("span_id", ...) call would (see
// encodeRecord) — so every existing `logger.WithContext(ctx)` call site in a
// handler wrapped by the interceptor starts correlating automatically, no
// caller changes needed. A no-op (returns l unchanged) if ctx carries no span
// context — outside an RPC handler, or a service that hasn't wired the
// interceptor into RegisterRPC yet.
func (l *OTelLogger) WithContext(ctx context.Context) Logger {
	traceIDHex, spanIDHex, ok := spanFromContext(ctx)
	if !ok {
		return l
	}
	return l.WithFields(Fields{"trace_id": traceIDHex, "span_id": spanIDHex})
}

func (l *OTelLogger) WithField(key string, value any) Logger {
	return l.WithFields(Fields{key: value})
}

func (l *OTelLogger) WithFields(fields Fields) Logger {
	merged := make(Fields, len(l.fields)+len(fields))
	for k, v := range l.fields {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}
	return &OTelLogger{
		exporter:    l.exporter,
		serviceName: l.serviceName,
		level:       l.level,
		fields:      merged,
		depth:       l.depth,
	}
}

func (l *OTelLogger) WithCallDepth(depth int) Logger {
	return &OTelLogger{
		exporter:    l.exporter,
		serviceName: l.serviceName,
		level:       l.level,
		fields:      l.fields,
		depth:       depth,
	}
}

func (l *OTelLogger) Trace(msg string) { l.emit(TraceLevel, msg) }
func (l *OTelLogger) Debug(msg string) { l.emit(DebugLevel, msg) }
func (l *OTelLogger) Debugf(format string, args ...any) {
	l.emit(DebugLevel, fmt.Sprintf(format, args...))
}
func (l *OTelLogger) Info(msg string) { l.emit(InfoLevel, msg) }
func (l *OTelLogger) Infof(format string, args ...any) {
	l.emit(InfoLevel, fmt.Sprintf(format, args...))
}
func (l *OTelLogger) Warn(msg string) { l.emit(WarnLevel, msg) }
func (l *OTelLogger) Warnf(format string, args ...any) {
	l.emit(WarnLevel, fmt.Sprintf(format, args...))
}
func (l *OTelLogger) Error(msg string) { l.emit(ErrorLevel, msg) }
func (l *OTelLogger) Errorf(format string, args ...any) {
	l.emit(ErrorLevel, fmt.Sprintf(format, args...))
}

func (l *OTelLogger) WrappedError(err error, msg string) {
	e := Wrap(err, l.fields)
	n := l.WithFields(e.Fields()).WithField("error", e.Error()).(*OTelLogger)
	n.emit(ErrorLevel, msg)
}

// Fatal logs at FatalLevel then calls os.Exit(1). Unlike every other level,
// this uses the exporter's synchronous ExportSync rather than the async
// queue: os.Exit(1) runs no deferred code and would otherwise very likely
// race the background sender, losing the one log line an operator most needs
// to see.
func (l *OTelLogger) Fatal(msg string) {
	l.emitSync(FatalLevel, msg)
	os.Exit(1)
}

// Panic logs at PanicLevel (synchronously, for the same reason as Fatal —
// panic unwinds the stack immediately and may terminate the process before
// an async queue entry is ever sent) then panics.
func (l *OTelLogger) Panic(msg string) {
	l.emitSync(PanicLevel, msg)
	panic(msg)
}

// emit builds and (async, fire-and-forget) exports one LogRecord for msg at
// level, honoring the configured level threshold (see logger.go's LogLevel
// iota ordering: PanicLevel=0 is most severe, TraceLevel=6 is most verbose —
// a call is emitted when its level is at or more severe than the configured
// threshold). No-ops entirely before Start has been called or when the
// exporter is disabled.
func (l *OTelLogger) emit(level LogLevel, msg string) {
	if l.exporter == nil || !l.exporter.Enabled() || level > l.level {
		return
	}
	l.exporter.Export(logsExportProcedure, l.encodeRecord(level, msg))
}

// emitSync is emit's synchronous counterpart, used only by Fatal/Panic.
func (l *OTelLogger) emitSync(level LogLevel, msg string) {
	if l.exporter == nil || !l.exporter.Enabled() || level > l.level {
		return
	}
	l.exporter.ExportSync(logsExportProcedure, l.encodeRecord(level, msg))
}

// encodeRecord builds one Export request body for a single log record at
// level with message msg, including this logger's accumulated fields as
// OTLP attributes and, if present under the well-known "trace_id"/"span_id"
// field keys (hex strings — see WithContext's doc comment), the record's
// dedicated LogRecord.trace_id/span_id.
func (l *OTelLogger) encodeRecord(level LogLevel, msg string) []byte {
	attrs := make(map[string]string, len(l.fields))
	var traceIDHex, spanIDHex string
	for k, v := range l.fields {
		s := fmt.Sprintf("%v", v)
		switch k {
		case "trace_id":
			traceIDHex = s
		case "span_id":
			spanIDHex = s
		}
		attrs[k] = s
	}

	record := encodeLogRecord(logRecordInput{
		timeUnixNano:   uint64(time.Now().UnixNano()),
		severityNumber: severityNumberFor(level),
		severityText:   level.String(),
		body:           msg,
		attributes:     attrs,
		traceIDHex:     traceIDHex,
		spanIDHex:      spanIDHex,
	})
	return encodeExportLogsRequest(l.serviceName, [][]byte{record})
}
