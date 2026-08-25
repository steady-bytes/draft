package main

import (
	"context"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	authv1 "github.com/steady-bytes/draft/api/examples/auth/v1"
	authv1Connect "github.com/steady-bytes/draft/api/examples/auth/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

func main() {
	chassis.NewMetricsReporter().Start()
	var (
		logger = chassis.NewOTelLogger()
		ctrl   = &controller{logger: logger}
	)

	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "examples",
		}).
		WithRPCHandler(ctrl).
		// Public route — anyone can call Greet, no token required.
		WithRoute(&ntv1.Route{
			Name: "auth-example-greet",
			Match: &ntv1.RouteMatch{
				Prefix: "/examples.auth.v1.AuthExampleService/Greet",
			},
			Auth: &ntv1.RouteAuth{
				Policy: ntv1.AuthPolicy_AUTH_POLICY_BYPASS,
			},
		}).
		// Protected route — caller must present a valid bearer token.
		WithRoute(&ntv1.Route{
			Name: "auth-example-secret",
			Match: &ntv1.RouteMatch{
				Prefix: "/examples.auth.v1.AuthExampleService/Secret",
			},
			Auth: &ntv1.RouteAuth{
				Enabled: true,
				Policy:  ntv1.AuthPolicy_AUTH_POLICY_AUTHENTICATED,
			},
		}).
		Start()
}

type controller struct {
	logger chassis.Logger
}

func (c *controller) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := authv1Connect.NewAuthExampleServiceHandler(c, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, handler, true)
}

// Greet is public — no auth required by Envoy so this always succeeds.
func (c *controller) Greet(ctx context.Context, req *connect.Request[authv1.GreetRequest]) (*connect.Response[authv1.GreetResponse], error) {
	name := req.Msg.Name
	if name == "" {
		name = "stranger"
	}
	return connect.NewResponse(&authv1.GreetResponse{
		Message: "Hello, " + name + "! (public endpoint)",
	}), nil
}

// Secret requires a valid token. If the auth service is in bypass mode every
// request reaches here; in authentik mode only authenticated callers do.
// The X-Authentik-Username header is set by the auth service on allow.
func (c *controller) Secret(ctx context.Context, req *connect.Request[authv1.SecretRequest]) (*connect.Response[authv1.SecretResponse], error) {
	caller := req.Header().Get("X-Authentik-Username")
	if caller == "" {
		caller = "anonymous (auth bypass mode)"
	}
	return connect.NewResponse(&authv1.SecretResponse{
		Caller: caller,
		Secret: "the launch codes are: draft-auth-works",
	}), nil
}
