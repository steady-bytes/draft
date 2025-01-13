module github.com/steady-bytes/draft/tools/blueprint-client

go 1.21.3

replace github.com/steady-bytes/draft/api => ../../api

require (
	connectrpc.com/connect v1.16.2
	github.com/google/uuid v1.6.0
	github.com/steady-bytes/draft/api v0.0.1
	golang.org/x/net v0.23.0
	google.golang.org/protobuf v1.34.2
)

require golang.org/x/text v0.14.0 // indirect
