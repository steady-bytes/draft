package main

import (
	c "github.com/steady-bytes/draft/internal/host/controller"
	h "github.com/steady-bytes/draft/internal/host/handler"
	m "github.com/steady-bytes/draft/internal/host/model"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

func main() {
	var (
		name            = "host"
		testModel       m.TestModel
		testCtrl        c.TestController
		testHTTPHandler h.TestHTTPHandler
		testRPCHandler  h.TestRPCHandler
	)

	testModel = m.NewTestModel()
	testCtrl = c.NewTestCtrl(testModel)
	testHTTPHandler = h.NewTestView(testCtrl)
	testRPCHandler = h.NewTestRPCHandler()

	defer draft.New(name, "").
		WithRepo(draft.PostgresBun, testModel).
		WithHTTPHandler(testHTTPHandler).
		WithRPCHandler(testRPCHandler).
		// WithConsumer().
		// WithProducer().
		Start()
}
