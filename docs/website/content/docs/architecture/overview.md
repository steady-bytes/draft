---
weight: 2
title: "Overview"
description: "Overview of the **Draft** system design"
icon: "code"
draft: false
toc: true
---

# **Draft**

Draft is an open source framework for building distributed systems. With the popularity of [service-oriented architectures (SOAs)](https://en.wikipedia.org/wiki/Service-oriented_architecture) and [microservices](https://en.wikipedia.org/wiki/Microservices) it seemed fruitful to build a framework that would establish a path to building reliable systems and simplify a complex problem space.

{{< alert context="info" text="A few things to acknowledge before reading further. **SOAs** and **Microservices** are distributed systems and distributed systems should not be implemented without a real need. If you can scale vertically do that first. If your application is resource hungry then figure out why and optimize that first. Finally, if you have a large team and you want to functionally break up the system because of your team composition solve this outside of code. You've been warned, now let's go have fun building." />}}

Draft consists of a few core services that will run with your application, a command line tool known as `dctl` or [(draft controller)](https://github.com/steady-bytes/draft/tree/main/tools/dctl), and some Go modules that can be used to build services.


## Core Services

- [Blueprint](https://github.com/steady-bytes/draft/tree/main/services/core/blueprint)- A distributed key/value store and service registry for handling service discovery.
- [Fuse](https://github.com/steady-bytes/draft/tree/main/services/core/fuse) - A [control plane](https://en.wikipedia.org/wiki/Control_plane) handling routing configuration to services running within Draft systems.
- [Catalyst](https://github.com/steady-bytes/draft/tree/main/services/core/catalyst) - An event streaming interface for real-time message production and consumption.

## Service Patterns

Each service is built following the [microservice chassis](https://microservices.io/patterns/microservice-chassis.html) pattern. This gives the services (also known as processes) a common and solid foundation to build your application code on. Our primary intent in using the chassis pattern is be able to offload certain reusable concerns from the service without having to rely on additional infrastructure to augment a service (i.e using a sidecar pattern to handle circuit breakers in network traffic). This reduces system complexity and operational overhead.

Additionally, the integration with system dependencies (i.e databases, caches, or additional common libraries/SDKs) is most of the time handled at the chassis level and injected into the application as needed. A primary example of this is RPC servers and clients. The chassis interface exposes an `RPCer` interface that allows the application service to register the implementation of the RPC service with the chassis which in turn handles registration and port binding. We will discuss the full implementation of this in more detail, but this design enables better testing, system consistency, and a clear separation of concerns.

Services can easily be built using the [model-view-controller](https://en.wikipedia.org/wiki/Model%E2%80%93view%E2%80%93controller) pattern for each API service definition it implements. A great example of this is [blueprint](https://github.com/steady-bytes/draft/tree/start-arch-docs/services/core/blueprint) that implements a key/value database interface within the `key_value` package. Below is an example of what the file structure looks like. We will review the the construction of services in much more detail throughout the documentation.

```sh
blueprint
└── key_value
    ├── controller.go
    ├── model.go
    └── rpc.go # view
├── main.go
├── go.mod
└── go.sum
```

{{< alert context="info" text="The `rpc.go` file is considered the view in this service. Because it offers an interface into the data and functionality the `key_value` package is the owner of." />}}

## Dependencies

Draft is designed to minimize the usage of system dependencies as much as possible. Where needed, core functionality is built into Draft components to reduce operational overhead. Additionally, the deployment of a Draft system is not coupled to any one deployment method. Draft systems can be orchestrated using [Kubernetes](https://kubernetes.io/),  [Nomad](https://www.nomadproject.io/), run on bare-metal or deployed with some hybrid approach. It's up to the operator (you) to determine what orchestration best meets the needs of the system. Over the next few pages we will review the Draft framework and walk you through building an application using Draft.
