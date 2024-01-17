package main

import (
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

const (
	NAME = "file_host"
)

func main() {
	var (
	// testModel m.TestModel
	// testCtrl  c.TestController
	// testHTTPHandler h.TestHTTPHandler
	// testRPCHandler  h.TestRPCHandler
	)

	// testModel = m.NewTestModel()
	// testCtrl = c.NewTestCtrl(testModel)
	// testHTTPHandler = h.NewTestView(testCtrl)
	// testRPCHandler = h.NewTestRPCHandler()

	defer draft.New(NAME, "").
		// WithRepo(draft.PostgresBun, testModel).
		// WithHTTPHandler(testHTTPHandler).
		// WithRPCHandler(testRPCHandler).
		// WithConsumer().
		// WithProducer().
		Start()
}
