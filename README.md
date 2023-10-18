## TODO:
* Adding super token integration
	- manual super token integration
	- deploy super token
	- add super token to backend
	- integrate super token to db

# Draft
A framework for building reliable, efficient, scalable, and real-time distributed systems.

## Components of draft

* Gateway:
	The public entrypoint to the system. This component uses the `envoy` proxy with either static, or dynamic
	configuration.

* Control Plane:
	The dynamic controller of `envoy` 

* Service Registry: 
	A key/value storage that also validates service are alive and serving traffic. This gives the system a single pane
	of glass to pin point specific failures or running state of the system. Routing is also configured with the `Control Plane`
	so that traffic can be routed to the accepting service.

* Host: 
	- Authentication: The user authentication interface.
	- FileHost: A host for static assets, public or private.

* Command Handler: The interface to invoke a command, or a write to the system.
* Query Handler: The interface to invoke a query, or a read from the system.

* Event Store: 
	The means to which each event is `emitted` (stored, and forwarded). It's a similar concept to the
	write ahead log for all the events in the system. The underlying storage for now will be scylla db.

* Subscriber Gateway: 
	The public entrypoint for a client to consume public events. The subscriber_gateway authenticates user
	request to connect, and handles the persistent connect between the client, and server.

* CLI:
	A cli tool for building, operating, and testing the components of draft.

## Storage Components
In an effort to increase the separation of concerns throughout the whole system. Each supported data store will have it's own
implementation for `Insert`, `Select`, `Update`, and `Delete` operations. The general idea is to consume different events to
perform different storage operations. A central tenant of `Draft` is to keep things simple, and avoid complexity in any way
possible.

* Inserter: A consumer service that is responsible for writing data to a specific type of database
	- postgres
	- scylla
	- tikv
	- clickhouse

* Selector: A consumer service that is responsible for resolving quires for integrated databases
	- postgres
	- scylla
	- tikv
	- clickhouse

* Updater:
	- postgres
	- scylla
	- tikv
	- clickhouse

* Deleter: A consumer service that is responsible for finding and deleting values by it's primary key

## Api/src/draft
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

## Internal
Each directory is a self contained implementation of one of the system components. Each component is a cli command that
can be executed on it's own, or in a greater environment. For each component command the draft runtime is invoked meaning
it's capable of reading static configuration, and binding to os sockets. The goal of this design is to make each service
testable, composable, and responsible for only one thing.

## User Authentication


## Pkg
Contains internal reusable packages that different components of the system can share. A good example of something that might
find a home in `pkg` is an `authorization` client.
