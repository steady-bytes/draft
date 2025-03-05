# Draft

Draft is a framework for easily building reliable, efficient, and scalable distributed systems. It contains all the necessary building blocks like a microservice chassis, service discovery, database connections, messaging and eventing systems, and much more. Building a good distributed system shouldn't be hard. Draft makes it easy by providing a set of tools that make building distributed systems easier.

## Components of Draft

### Blueprint

Blueprint is a core service that provides both a service registry and a generic key/value store for dynamic service configuration. All processes in a Draft cluster register themselves with Blueprint at startup so that Blueprint can manage all service configuration and provide a single view into the status of the cluster.

### Catalyst
Catalyst is a core service that acts as the primary message broker, and actor system. Services, and clients can `Produce` and `Consume` [CloudEvent](https://cloudevents.io/) messages. Currently a simple `Broadcast` message delivery is implemented so each `Consumer` of the same channel will receive the message at the same time.

### Fuse

Fuse is a core service that enabled routing between Draft processes as well as ingress into the cluster. It manages an installation of [Envoy](https://www.envoyproxy.io/) to route requests from clients to services.

### Chassis

The Draft Chassis provides all the necessary pieces to create Draft processes (services). It is a Go module that handles service registration to Blueprint, configuration management, graceful shutdowns, messaging protocols, and much more. The Chassis also implements a plugin system to support any type of required integration. Many of these are supported out-of-the-box like Postgres or NATS, but you can also implement your own. These plugins can be used to integration with custom providers for message brokers, loggers, databases, secret vaults, and more.

### dctl

`dctl` is a command line tool that provides a set of commands to simplify working with a Draft system. It performs tasks like building protobufs, running services locally, and managing local infrastructure.


## Project Structure

Here are some simple descriptions of what you'll find in each directory:

- `api`: Definitions of data structures, RPC interfaces, event specifications, and more.
- `deployments`: Configuration for deploying the Draft cluster and (optionally) its external service dependencies.
- `pipelines`: Definitions of pipelines (CI/CD) which build, test, and deploy Draft.
- `pkg`: Reusable packages that are shared between services. For example, the Draft Chassis which is used by all Golang-based Draft services.
- `services`: Service code grouped into domains. For example, the `services/core/` contains the code for the core Draft services Blueprint and Fuse.
- `tests`: Functional tests used to make sure features are functioning with each release end to end.
- `tools`: Clients and tooling for working on Draft systems. For example, the Draft CLI (`dctl`).


## Getting Started

### Prerequisites

* [Go](https://golang.org/doc/install) v1.21 (we suggest using [gvm](https://github.com/moovweb/gvm) for easier version management)
* [Docker](https://docs.docker.com/get-docker/)
* [Kubernetes](https://kubernetes.io/docs/tasks/tools/) (this is for running testing suites locally; if on Mac or Windows you can use the Kubernetes engine built into Docker Desktop)

You'll need the `dctl` CLI tool to work with everything in Draft. Let's install it now:

```shell
go install github.com/steady-bytes/draft/tools/dctl@latest
```

We'll need to import this project as a usable context into `dctl` so it can manage things for us. After cloning the repo run the below command from inside the repo:

```shell
dctl context import
```

Now you can set up your local environment:

```shell
# initialize and start local infrastructure using Docker
dctl infra init
dctl infra start

# test run some domains
dctl run --domains examples

# initialize and do a first generation of the API
dctl api init
dctl api build
```

## Future Components

### Foundation

* __Content Delivery Network__: Most applications need to store files from `.pdf`'s to `.mov` files. The file host will control the life cycle of assets that will be used by the system.

## Future Ideas

* __Command Handler__: The interface to invoke a command, or a write to the system.
* __Query Handler__: The interface to gather information from the system
* __Event Store__: The means to which each event is `emitted` (stored, and forwarded). It's a similar concept to the write ahead log for all the events in the system. The underlying storage facility has yet to be determined, however `ScyllaDB` or `ClickHouse` are the first two in the running. I like the idea of `ClickHouse` because the system will most likely already have an instance of this because of what will be used for logging. It's basically a wrapper around a message bus, or message queue. In the case of draft we are using `red panda` it's a kafka replacement rewritten in `c++`.

## Integrated Components (future)

* __Application Router__: The public entrypoint to the system. This component uses the `envoy` proxy with either static, or dynamic configuration.
* __Observability System__: Infrastructure, and configuration for monitoring the running system.
