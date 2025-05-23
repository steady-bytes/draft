package main

import (
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"

	cp "github.com/steady-bytes/draft/services/core/fuse/control_plane"
)

func main() {
	var (
		logger       = zerolog.New()
		controlPlane = cp.NewControlPlane(logger)
		// xDS server containing a share cache between the envoy proxies
		xdsServer = cp.NewXDSRpc(logger, controlPlane)
		// fuse control plane rpc interface
		controlPlaneRPC = cp.NewRPC(logger, controlPlane)
	)

	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "core",
		}).
		WithRPCHandler(xdsServer).
		WithRPCHandler(controlPlaneRPC).
		// make sure to load the cache on boot
		WithRunner(controlPlane.LoadCache).
		Start()
}
