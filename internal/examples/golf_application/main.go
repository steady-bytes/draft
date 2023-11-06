package main

func main() {

	defer draft.
		// system environment is consumed
		Init().
		// database dependencies
		// inner most layer gets created first
		WithRepo().
		// business logic
		WithController().
		// external protocols
		// outer most layer of the service
		// the way of exposing behavior to the external world
		WithRPC().
		WithConsumer().
		WithHTTP().
		// run the server, until it was killed
		// by the environment
		Start()
}
