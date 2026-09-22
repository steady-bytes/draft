// controller.go: the shared state every domain file (objective.go, task.go,
// agent.go, scheduler.go, loop.go) hangs its methods off of. Thin model/
// controller/rpc split mirroring services/core/blueprint/key_value's own
// file separation: this package holds business logic; rpc.go is the only
// thing that speaks connect.Request/Response and telemetry spans.
package main

import (
	"time"

	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"
)

type controller struct {
	logger         chassis.Logger
	kv             kvv1Connect.KeyValueServiceClient
	events         EventPublisher
	watchers       *watchBroadcaster
	tickerInterval time.Duration
}

func newController(logger chassis.Logger, kv kvv1Connect.KeyValueServiceClient, events EventPublisher, tickerInterval time.Duration) *controller {
	return &controller{
		logger:         logger,
		kv:             kv,
		events:         events,
		watchers:       newWatchBroadcaster(),
		tickerInterval: tickerInterval,
	}
}
