---
weight: 2
title: "Architecture"
description: "Overview of the **Draft** system design"
icon: "code"
draft: false
toc: true
---

# **Draft**
An open source framework for building distributed systems. With the popularity of [Service-Oriented architectures](https://en.wikipedia.org/wiki/Service-oriented_architecture) and [Microservices](https://en.wikipedia.org/wiki/Microservices) it seemed fruitful to put together a framework that would establish a path to building reliable systems and hopefully help simplify a complex problem space.

> A few things to acknowledge before reading further. **SOA's** and **Microservices** are distributed systems and distributed systems should not be implemented without a real need. If you can scale vertically do that first. If your application is resource hungry then figure out why, and optimize that first. Finally, if you have a large team and you want to functionally break up the system because of your team composition. Solve this outside of code. You've been warned, now let's go have fun building.

Draft consists of a few core services that will run with your application, a command line tool known as `dctl` or [(draft command line tool)](https://github.com/steady-bytes/draft/tree/main/tools/dctl), and some `GoLang` modules that can be used to build services.


## Core Services

- [Blueprint](https://github.com/steady-bytes/draft/tree/main/services/core/blueprint)- A distributed `key/value` store, and service registry handling service discovery.
- [Fuse](https://github.com/steady-bytes/draft/tree/main/services/core/fuse) - A [control plane](https://en.wikipedia.org/wiki/Control_plane) handling routing configuration and decisions to services running within the framework.
- [Catalyst](https://github.com/steady-bytes/draft/tree/main/services/core/catalyst) - An event streaming `interface` for real-time message production, and consumption.

## Service Patterns

Each service is built following the [Microservice chassis](https://microservices.io/patterns/microservice-chassis.html) pattern. This gives the service/processes a common and solid foundation to build your application code on. Our primary intent in using the `chassis` pattern is be able to offload certain reusable concerns to the service without having to rely on additional infrastructure to augment a service (i.e using a side car pattern to handle circuit breakers in network traffic).

Additionally, the integration with system dependencies (i.e database, cache, or additional service libs/sdk's) is most of the time handled at the chassis level, and injected into the application that may need it. A primary example of this is `gRPC` `servers` and `clients`. The `chassis` interface exposes a `RPCer` interface that allows the application service to register the implementation of the rpc service with the chassis to expose on the consumed port. We will discuss the full implementation of this in more detail but this design lends itself to allow for better testing, more consistency throughout the system, and a clear separation of concerns.

Finally, a service follows the `MVC` pattern for each `api` service definition it implements. A great example of this is [Blueprint]() that implements the `key/value` interface within the `key_value` module of the `microservice`. Below is an example of what the structure looks like. We will review the the construction of services in much more detail throughout the documentation.

```sh
blueprint
└── key_value
    ├── controller.go
    ├── model.go
    └── rpc.go #
```
> **NOTE:** The `rpc.go` file is considered the view in this service. Because it offers an interface into the data the `key_value` module is the owner of.

## Dependencies
System dependencies are taken on only as the optimal solution. We have put a lot of effort into managing system dependencies and reducing them as much as possible. Additionally, the deployment of a `draft` system is not coupled to any one orchestration system. [Kubernetes](https://kubernetes.io/), and [Nomad](https://www.nomadproject.io/) are incredible tools and amazing at what they do. However, a fine line must be taken as `draft` services can be running anywhere. A few VM's, multiple different containers around the world or a network of embedded systems. It's up to the operator(you) to determine what your orchestration later will look like.