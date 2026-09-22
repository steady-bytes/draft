package key_value

// caller_service.go carries "which service is pushing this write" through a
// Set/Delete call's context, so controller.go's write-path spans and logs
// (see Set/Delete in controller.go) can be attributed to the right caller
// without threading an extra string parameter through every call site.
//
// Two producers populate it today:
//   - rpc.go's Set/Delete handlers, from the inbound chassis.CallerServiceHeaderName
//     header (set automatically by chassis.NewCallerServiceClientInterceptor for any
//     client that uses it).
//   - service_discovery/controller.go's Synchronize/Initialize/Finalize/Reap, from the
//     process registry's own Name field (already known there without a header).

import "context"

type callerServiceContextKey struct{}

// WithCallerService returns a copy of ctx carrying name as the write's caller-service
// attribution. An empty name is stored as-is (CallerServiceFromContext then reports "").
func WithCallerService(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, callerServiceContextKey{}, name)
}

// CallerServiceFromContext returns the caller-service name ctx carries, or "" if none
// was ever set (e.g. a Set/Delete call that bypassed WithCallerService entirely).
func CallerServiceFromContext(ctx context.Context) string {
	name, _ := ctx.Value(callerServiceContextKey{}).(string)
	return name
}
