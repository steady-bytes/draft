package ingest

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/query"
	"github.com/steady-bytes/draft/services/core/beacon/store"
)

// serviceNameAttrKey is the OTel semantic-conventions resource attribute key
// carrying a process's declared service name.
const serviceNameAttrKey = "service.name"

// Receiver implements opentelemetry.proto.collector.logs.v1.LogsService/Export
// directly against go.opentelemetry.io/proto/otlp's generated message/gRPC
// types — no collector/receiver framework, matching how Catalyst hand-rolls its
// own batching rather than pulling in a library (see the design doc's Service
// Architecture section).
type Receiver struct {
	collectorlogsv1.UnimplementedLogsServiceServer

	writer    *Writer
	publisher query.Publisher
	logger    chassis.Logger
}

func NewLogsReceiver(writer *Writer, publisher query.Publisher, logger chassis.Logger) *Receiver {
	return &Receiver{writer: writer, publisher: publisher, logger: logger}
}

func (r *Receiver) Export(_ context.Context, req *collectorlogsv1.ExportLogsServiceRequest) (*collectorlogsv1.ExportLogsServiceResponse, error) {
	var accepted int

	for _, rl := range req.GetResourceLogs() {
		resourceAttrs := attrsToMap(rl.GetResource().GetAttributes())
		serviceName := resourceAttrs[serviceNameAttrKey]

		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				row := logRecordToRow(lr, serviceName, resourceAttrs)
				r.writer.Save(row)
				r.publisher.Publish(row)
				accepted++
			}
		}
	}

	r.logger.WithField("accepted", accepted).Trace("accepted OTLP log export")
	return &collectorlogsv1.ExportLogsServiceResponse{}, nil
}

func logRecordToRow(lr *logsv1.LogRecord, serviceName string, resourceAttrs map[string]string) store.LogRow {
	ts := lr.GetTimeUnixNano()
	if ts == 0 {
		ts = lr.GetObservedTimeUnixNano()
	}

	severityText := lr.GetSeverityText()
	if severityText == "" {
		severityText = lr.GetSeverityNumber().String()
	}

	return store.LogRow{
		Timestamp:          time.Unix(0, int64(ts)).UTC(),
		TraceID:            hex.EncodeToString(lr.GetTraceId()),
		SpanID:             hex.EncodeToString(lr.GetSpanId()),
		SeverityText:       severityText,
		SeverityNumber:     uint8(lr.GetSeverityNumber()),
		ServiceName:        serviceName,
		Body:               anyValueToString(lr.GetBody()),
		Attributes:         attrsToMap(lr.GetAttributes()),
		ResourceAttributes: resourceAttrs,
	}
}

func attrsToMap(kvs []*commonv1.KeyValue) map[string]string {
	out := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		out[kv.GetKey()] = anyValueToString(kv.GetValue())
	}
	return out
}

func anyValueToString(v *commonv1.AnyValue) string {
	if v == nil {
		return ""
	}
	switch val := v.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return val.StringValue
	case *commonv1.AnyValue_BoolValue:
		return fmt.Sprintf("%t", val.BoolValue)
	case *commonv1.AnyValue_IntValue:
		return fmt.Sprintf("%d", val.IntValue)
	case *commonv1.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", val.DoubleValue)
	case *commonv1.AnyValue_BytesValue:
		return hex.EncodeToString(val.BytesValue)
	case *commonv1.AnyValue_ArrayValue:
		out := make([]string, len(val.ArrayValue.GetValues()))
		for i, item := range val.ArrayValue.GetValues() {
			out[i] = anyValueToString(item)
		}
		return fmt.Sprintf("%v", out)
	case *commonv1.AnyValue_KvlistValue:
		return fmt.Sprintf("%v", attrsToMap(val.KvlistValue.GetValues()))
	default:
		return ""
	}
}
