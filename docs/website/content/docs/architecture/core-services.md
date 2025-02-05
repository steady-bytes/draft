---
weight: 3
title: "Core Services"
description: "Core services that make up a draft cluster"
icon: "code"
draft: false
toc: true
---

# Core Services

A draft cluster is made up of a few core services that are used to store your system configuration, handle service discovery, manage route tables, and produce, and consume events.

- [Blueprint](https://github.com/steady-bytes/draft/tree/main/services/core/blueprint)- A distributed key/value store and service registry for handling service discovery.
- [Fuse](https://github.com/steady-bytes/draft/tree/main/services/core/fuse) - A [control plane](https://en.wikipedia.org/wiki/Control_plane) handling routing configuration to services running within Draft systems.
- [Catalyst](https://github.com/steady-bytes/draft/tree/main/services/core/catalyst) - An event streaming interface for real-time message production and consumption.

Additionally, [envoy proxy]() is used as the ingress controller to the system. Envoy is controlled by routing information given to it by Fuse.

![Draft System](/draft-system.png)

### Blueprint
A distributed key/value store and service registry. Blueprint uses a number of algorithms to achieve write heavy performance while also maintaining it's data consistency between all nodes in the cluster. [Raft Consensus Algorithm](https://raft.github.io/) is use to establish a leader of the cluster and to maintain leadership within the cluster in case of a service failure or network partition. If the leader is lost to the cluster a new election begins and another leader is agreed upon. To keep Blueprints design simple only leader nodes can accept a write request. If a follower, or voter node receives a write request then it's forwarded to the leader with out the clients knowledge. Additionally the client is informed that it's attempting to write to a follower and receives the new leaders connection details. Next, Blueprint uses a combination of [LSM tree](https://en.wikipedia.org/wiki/Log-structured_merge-tree), and a [FSM](https://en.wikipedia.org/wiki/Finite-state_machine) to efficiently store data on the file system and guarantee each log/message will be written to all nodes of the cluster. This design gives the remaining system an efficient and reliable key/value storage layer.

The next responsibility of Blueprint is to handle the service registry, health, and status checks of registered processes. This is a loaded topic so I'm going to do my best to break it down in a simple way. Generally, I think of a service registry as the system journal or [SystemD](https://en.wikipedia.org/wiki/Systemd) of a distributed system. Currently, Blueprint is not responsible for scheduling additional resources in a failure case(maybe in the future) so it's main responsibility is to keep track of the state and details of registered [processes](https://github.com/steady-bytes/draft/blob/main/api/core/registry/service_discovery/v1/models.proto#L33). Two specific details are important.

* [Process Running State](https://github.com/steady-bytes/draft/blob/e7bb27cf10c60decbdb668cfbacf11ceadd9de86/api/core/registry/service_discovery/v1/models.proto#L88) - A value retained to determine if a process is ready for certain kinds of traffic.
* [Process Health State](https://github.com/steady-bytes/draft/blob/e7bb27cf10c60decbdb668cfbacf11ceadd9de86/api/core/registry/service_discovery/v1/models.proto#L96) - The healthy-ness state of the process.

### Process Registration
When a process starts and it's configured with an entrypoint. The process will begin the registration flow which is outlines below.

1. A sends a nonce to Blueprint
2. Blueprint will validate the nonce, create a Process Identity and add the process to the registry
3. The registering process will open a bi-directional gRPC stream to the registry pushing it's Running state, and Health state to the registry
4. The registry will update itself with the new details, and ack each message with Blueprint cluster details
5. When the process is ready to leave it will close the bi-directional stream and call finalize to be removed from the registry

![Process Registration](process-registration.png)

Once a process is registered other services can use the metadata to lookup services they might want to communicate with. For example the default behavior is to register the gRPC service definitions of a process. So if process B needs to talk to process A via gRPC then process B can establish a connection with A by looking it up in the registry.