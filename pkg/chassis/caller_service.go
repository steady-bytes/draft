package chassis

// caller_service.go lets an RPC handler recover which chassis service made an
// outbound call to it. It mirrors otel_trace.go's traceparent propagation (a
// client interceptor stamps a header, the server reads it back) but carries
// service identity instead of trace context.
//
// Motivating case: Blueprint's KeyValueService.Set/Delete handlers want to
// attribute every write to the service that made it (see
// services/core/blueprint/key_value/rpc.go and controller.go), but a plain
// SetRequest/DeleteRequest carries no caller identity today. Any chassis
// client wrapped with NewCallerServiceClientInterceptor closes that gap for
// whoever reads CallerServiceFromHeader server-side.

import (
	"context"

	"connectrpc.com/connect"
)

// CallerServiceHeaderName is the header NewCallerServiceClientInterceptor sets
// and CallerServiceFromHeader reads.
const CallerServiceHeaderName = "X-Draft-Caller-Service"

type callerServiceClientInterceptor struct {
	serviceName string
}

// NewCallerServiceClientInterceptor constructs a connect.Interceptor for a
// connect-go client that stamps every outbound call with this process's own
// service name (chassis.GetConfig().Name()), e.g.:
//
//	kvv1Connect.NewKeyValueServiceClient(httpClient, entrypoint,
//		connect.WithInterceptors(chassis.NewCallerServiceClientInterceptor()))
//
// The callee recovers it with CallerServiceFromHeader.
func NewCallerServiceClientInterceptor() connect.Interceptor {
	return callerServiceClientInterceptor{serviceName: GetConfig().Name()}
}

func (i callerServiceClientInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set(CallerServiceHeaderName, i.serviceName)
		return next(ctx, req)
	}
}

func (i callerServiceClientInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set(CallerServiceHeaderName, i.serviceName)
		return conn
	}
}

// WrapStreamingHandler is left as a pure passthrough: this interceptor
// instruments outbound calls this process makes as a client, not inbound
// handlers.
func (callerServiceClientInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// CallerServiceFromHeader returns the calling service's name from an inbound
// request's caller-service header, set by NewCallerServiceClientInterceptor —
// "" if absent (a caller not using the interceptor, or one whose identity is
// established some other way, e.g. Blueprint's own service registry).
func CallerServiceFromHeader(h interface{ Get(string) string }) string {
	return h.Get(CallerServiceHeaderName)
}
