module blueprint_client

go 1.21.5

replace (
	github.com/steady-bytes/draft/api => ../../api/gen/go
)

require (
	connectrpc.com/connect v1.14.0
	github.com/dgraph-io/badger/v2 v2.2007.4
	github.com/hashicorp/raft v1.6.0
	github.com/steady-bytes/draft/api v0.0.1
)