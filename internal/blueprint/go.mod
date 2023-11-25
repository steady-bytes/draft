module github.com/steady-bytes/draft/blueprint

go 1.21.3

replace (
	github.com/steady-bytes/draft/api/gen/go v0.0.1 => ../../api/gen/go
	github.com/steady-bytes/draft/pkg/draft-runtime-golang v0.0.1 => ../../pkg/draft-runtime-golang
)

require (
	github.com/dn365/gin-zerolog v0.0.0-20171227063204-b43714b00db1
	github.com/gin-contrib/cors v1.4.0
	github.com/gin-gonic/gin v1.9.1
	github.com/rs/zerolog v1.31.0
	github.com/steady-bytes/draft/api/gen/go v0.0.1
	github.com/steady-bytes/draft/pkg/draft-runtime-golang v0.0.1
	github.com/supertokens/supertokens-golang v0.14.0
	github.com/uptrace/bun v1.1.16
	google.golang.org/grpc v1.59.0
)

require (
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/dgraph-io/badger/v2 v2.2007.4 // indirect
	github.com/dgraph-io/ristretto v0.0.3-0.20200630154024-f66de99634de // indirect
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/golang/snappy v0.0.3 // indirect
	github.com/klauspost/compress v1.17.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/net v0.14.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)
