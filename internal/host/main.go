package main

import (
	c "github.com/steady-bytes/draft/internal/host/controller"
	h "github.com/steady-bytes/draft/internal/host/handler"
	m "github.com/steady-bytes/draft/internal/host/model"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

func main() {
	var (
		model           m.TestModel
		ctrl            c.TestController
		testHTTPHandler h.TestHTTPHandler
		testRPCHandler  h.TestRPCHandler
	)

	model = m.NewTestModel()
	ctrl = c.New(model)
	testHTTPHandler = h.NewTestView(ctrl)
	testRPCHandler = h.NewTestRPCHandler()

	defer draft.New("host").
		WithRepo(draft.PostgresBun, model).
		WithHTTPHandler(draft.Gin, testHTTPHandler).
		WithRPCHandler(draft.Grpc, testRPCHandler).
		Start()
}
