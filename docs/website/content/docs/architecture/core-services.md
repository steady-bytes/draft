---
weight: 3
title: "Core Services"
description: "Core services that make up a draft cluster"
icon: "code"
draft: false
toc: true
---

# Core Services

A draft cluster is made up of a few core services that are used to store your system configuration, handle service discovery, manage route tables, and stream events.

- [Blueprint](https://github.com/steady-bytes/draft/tree/main/services/core/blueprint)- A distributed key/value store and service registry for handling service discovery.
- [Fuse](https://github.com/steady-bytes/draft/tree/main/services/core/fuse) - A [control plane](https://en.wikipedia.org/wiki/Control_plane) handling routing configuration to services running.
- [Catalyst](https://github.com/steady-bytes/draft/tree/main/services/core/catalyst) - An event streaming interface for real-time message production and consumption.

Additionally, [envoy proxy]() is used as the ingress controller to the system. Envoy is controlled by routing information given to it by Fuse.

</br>
<img src="/images/docs/draft-system.png" alt="draft system diagram overview" style="display:block; width:40%;"/>
</br>

### Blueprint
A distributed key/value store and service registry. Blueprint uses a number of algorithms to achieve write heavy performance while also maintaining it's data consistency between all nodes in the cluster. [Raft Consensus Algorithm](https://raft.github.io/) is use to establish a leader of the cluster and to maintain quorum between the cluster nodes. Gracefully handling nodes failure and network partitions. If the leader is lost to the cluster a new election begins and another leader is agreed upon. To keep Blueprints design simple, only one node (the leader) can accept a write request and write it to the LSM tree. If a follower, or voter node receives a write request then it's forwarded to the leader with out the clients knowledge. Additionally the client is informed that it's attempting to write to a follower and receives the new leaders connection details. Next, Blueprint uses a combination of [LSM tree](https://en.wikipedia.org/wiki/Log-structured_merge-tree), and a [FSM](https://en.wikipedia.org/wiki/Finite-state_machine) to efficiently store data on the file system and guarantee each log/message will be written to all nodes of the cluster. This design gives the remaining system an efficient and reliable key/value storage layer.

The next responsibility of Blueprint is to handle the service registry, health, and wellness checks of registered processes. This is a loaded topic so I'm going to do my best to break it down in a simple way. Generally, I think of a service registry as the system journal or [SystemD](https://en.wikipedia.org/wiki/Systemd) of a distributed system. Currently, Blueprint is not responsible for scheduling additional resources in a failure case (maybe in the future) so it's main responsibility is to keep track of the state and details of registered [processes](https://github.com/steady-bytes/draft/blob/main/api/core/registry/service_discovery/v1/models.proto#L33). Two specific details are important and kept up to date.

* [Process Running State](https://github.com/steady-bytes/draft/blob/e7bb27cf10c60decbdb668cfbacf11ceadd9de86/api/core/registry/service_discovery/v1/models.proto#L88) - A value retained to determine if a process is ready for certain kinds of traffic.
* [Process Health State](https://github.com/steady-bytes/draft/blob/e7bb27cf10c60decbdb668cfbacf11ceadd9de86/api/core/registry/service_discovery/v1/models.proto#L96) - The healthy-ness state of the process.

### Process Registration
When a process starts and it's configured with an entrypoint. The process will begin the registration flow which is outlines below.

1. A new Process sends a nonce to Blueprint
2. Blueprint will validate the nonce, create a Process Identity and add the process to the registry
3. The registering process will open a bi-directional gRPC stream to the registry pushing it's Running state, and Health state to the registry every 5 seconds by default
4. The registry will update itself with the new details, and ack each message with Blueprint cluster details
5. When the process is ready to leave it will close the bi-directional stream and call finalize to be removed from the registry

</br>
<img src="/images/docs/draft-process-registration.png" alt="Blueprint process registration diagram" style="display:block; width:25%;"/>
</br>

Once a process is registered other services can use the metadata to lookup services they might want to communicate with. For example the default behavior is to register the gRPC service definitions of a process. So if process B needs to talk to process A via gRPC then process B can establish a connection with A by looking it up in the registry.

### Fuse
The [control plane](https://en.wikipedia.org/wiki/Control_plane) for a draft cluster. When a service/process would like to expose routes to the outside world then a route can be registered with fuse, stored locally or in draft for a high availability persistence layer. Currently we only support [envy proxy](https://www.envoyproxy.io/) as the ingress proxy however this in the future could change. Below is how a route is established in the control plane.

1. Once a process has registered to Blueprint, the Fuse address is looked up, and routing details are sent to fuse.
2. Fuse will update envoy with the new route table
3. Optionally Fuse will store the routing information to Blueprint

</br>
<img src="/images/docs/fuse-route-registration.png" alt="Blueprint process registration diagram" style="display:block; width:25%;"/>
</br>

### Catalyst
The event interface of the draft system. Catalyst can be run in two ways. First, as a stand alone message bus that will handle event processing for all consumers and producers. Lastly, it can also integrate with other message systems like [kafka](https://kafka.apache.org/)/[red panda](https://www.redpanda.com/), and [nats](https://nats.io/). We are in early development of this component. If your interested in on collaborating with use please reach out.