package chassis

import (
	"context"
)

// Effect pairs a name (used for logging and debugging) with the closure that reverts
// whatever the effect did. Effects are tracked on Runtime in registration order and
// reverted in reverse (LIFO) order during shutdown, so a later effect that assumed an
// earlier one was already in place is guaranteed to be torn down first.
type Effect struct {
	Name    string
	Dispose func(context.Context) error
}

// Effect performs setup and, if it succeeds and returns a non-nil dispose, tracks that
// dispose on the runtime's effect stack so it runs during shutdown.
//
// setup returning a nil dispose is valid and means "this mutation has nothing to
// revert" (e.g. a SecretStore that has no Close). setup returning a non-nil error is
// treated the same way plugin setup failures always have been in chassis: fatal. A
// service that cannot establish a declared dependency at boot is not allowed to limp
// along, and nothing is registered on the effect stack for a setup that never
// completed — there would be nothing to revert.
//
// Effect is the primitive that WithRepository, WithBroker, and WithSecretStore are
// built on (see builder.go), and is also meant to be called directly for any other
// mutation that needs a tracked inverse — e.g. a KV write to Blueprint that should be
// retracted on graceful shutdown. See docs/website/content/docs/architecture/
// chassis-composability.md for the full design rationale.
func (c *Runtime) Effect(name string, setup func() (dispose func(context.Context) error, err error)) *Runtime {
	dispose, err := setup()
	if err != nil {
		c.logger.WithError(err).WithField("effect", name).Fatal("failed to apply effect")
	}
	if dispose != nil {
		c.effectsMu.Lock()
		c.effects = append(c.effects, Effect{Name: name, Dispose: dispose})
		c.effectsMu.Unlock()
	}
	return c
}
