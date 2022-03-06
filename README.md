# Firegraph
A graph database using the relational model as a storage solution.

## Project Structure
```sh
├── api
│   ├── buf.gen.yaml
│   ├── buf.lock
│   ├── buf.yaml
│   ├── Dockerfile
│   ├── gen
│   │   ├── docs
│   │   │   ├── docs.md
│   │   │   └── index.html
│   │   └── go
│   │       ├── event_store_grpc.pb.go
│   │       ├── event_store.pb.go
│   │       ├── event_store.pb.gorm.go
│   │       ├── gorm
│   │       │   └── gorm.pb.go
│   │       └── querier.pb.go
│   ├── go.mod
│   ├── GOPATH
│   ├── go.sum
│   ├── Makefile
│   ├── README.md
│   ├── src
│   │   ├── event_store.proto
│   │   ├── gateway.proto
│   │   ├── querier.proto
│   │   └── writer.proto
│   ├── tools.go
│   └── vendor
│       └── gorm
│           └── gorm.proto
├── cmd
│   ├── root.go
│   └── serve.go
├── deployments
├── docs
├── go.mod
├── go.sum
├── internal
│   ├── event_store
│   │   ├── controller.go
│   │   ├── model.go
│   │   └── rpc.go
│   ├── gateway
│   ├── querier
│   └── writer
├── LICENSE.md
├── main.go
└── README.md
```

## Api
Definitions of `RPC` interfaces, over the wire request/response message types, events, `validation` interface, and 
`Aggregate` structures/models. We may also create our `AST` types so they can be sent over the wire efficiently 
and used in many different languages.

The `/api` directory contains it's own `Makefile` that contains a few `targets` for code generation, and
environment setup. A `Dockerfile` is provided to serve as the code gen build agent, it can be run in a CI/CD
environment, or locally on your machine if you have docker installed. Right, now it's the responsibility for the
developer to check-in generated code, and we will use the local file system replace feature of `go` modules to 
consume generate code.

Run targets
```
# Build the docker image locally, and store in your local registry as apibuilder:v1
$ make compiler
```

Now that you have the build agent ready. You can compile the `go` code from our `proto`'s.
```
# if succesful the `protoc` compiler will be invoked with each plugin that has been configured in the `buf.gen.yaml`
# configuration file
$ make api
```

If you want to clean your generated code run the following.
```
# Clean generated code
$ make clean
```
## Cmd
The command directory is the `cli` input to the application. Each one of the system components can be executed from the same
binary.

For example if you would like run the `event_store` service then all you have to do is run the following.
```sh
$ firegraph event_store
```

Each process has defaults configurations if a argument/flag is __not__ set, or config file is __not__ used. So for example 
the `event_store` by default will run on port `50001`. If you want to change that then use the following arguments.

```sh
$ firegraph event_store --port 8080
# or, using shorthand
$ firegraph event_store -p 8080
```

This will run the `firegraph event_store` service on port `8080` instead.

## Internal
Each directory is a self contained implementation of one of the system components.

## Pkg
Contains internal reusable packages that different components of the system can share. A good example of something that might
find a home in `pkg` is an `authorization` client.
