# Draft
A framework for building reliable, efficient, scalable, real-time and stateful distributed systems.

## Components of draft

### Blueprint
* __Key/Value Store__: A distributed key/value service is available to store `env` vars that each process may need when they start up
* __Service Registry__: A place a `process` can register it's self too. Publicizing information about what is does and how it can be interacted with.

### [PRODUCT NAME]
* __Control Plane__: The main command center of the system as a whole. Not only is it the controller of `envoy`, a tight integration with `Blueprint` have been established so it's easy to consolidate operational information of the system. Features can also be activated, or deactivated through it's portal. 

### [PRODUCT NAME]
* __File Host__: Most applications need to store files from `.pdf`'s to `.mov` files. The file host will control the life
	cycle of assets that will be used by the system.

### [PRODUCT NAME]
* __Command Handler__: The interface to invoke a command, or a write to the system.
* __Query Handler__: The interface to gather information from the system

### [PRODUCT NAME]
* __Event Store__: The means to which each event is `emitted` (stored, and forwarded). It's a similar concept to the write ahead log for all the events in the system. The underlying storage facility has yet to be determined, however `ScyllaDB` or `ClickHouse` are the first two in the running. I like the idea of `ClickHouse` because the system will most likely already have an instance of this because of what will be used for logging. It's basically a wrapper around a message bus, or message queue. In the case of draft we are using `red panda` it's a kafka replacement rewritten in `c++`.

### Envoy
* __Application Router__: The public entrypoint to the system. This component uses the `envoy` proxy with either static, or dynamic configuration.

### [PRODUCT NAME] (TBD, will most likely use something already made)
* __Observability System__: Infrastructure, and configuration for monitoring the running system.

---

# Project Structure

## api/
Definitions of `RPC` interfaces, over the wire request/response message types, events, and internal models.

The `/api` directory contains it's own `Makefile` that contains a few `targets` for code generation, and
environment setup. A `Dockerfile` is provided to serve as the code gen build agent, it can be run in a CI/CD
environment, or locally on your machine if you have docker installed. Right, now it's the responsibility for the
developer to check-in generated code, and we will use the local file system replace feature of `go` modules to 
consume generate code.

Run targets
```
# Build the docker image locally, and store in your local registry as apibuilder:v1
$ make compiler

# Now that you have the code compiler/build agent ready. You can compile the `go` code from our `proto`'s.
# Invoke the below target to generate all of our api code
$ make api

# If you want to clean your generated code run the following.
$ make clean
```

## internal/
Each directory is a self contained implementation of one of the system components.
__Current List:__
1. Blueprint (in progress)
2. Healthz (done): A simple http example

## pkg/
Contains internal reusable packages that different components of the system can share. A good example of something that might
find a home in `pkg` is an `authorization` client.

## deployments/
The local, stage, and production deployment configuration of draft components, and external service dependencies. 

## tests/
Functional tests used to make sure features are functioning with each release end to end.

## tools/
Contains `draft-cli` or `dctl`. A tool for working on, and with systems that use draft as a framework.