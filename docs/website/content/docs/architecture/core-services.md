---
weight: 3
title: 'Core Services'
description: 'An overview of the core components of a Draft cluster'
icon: 'code'
draft: false
toc: true
---

A Draft cluster is made up of a few core services that are used to store your system configuration, handle service discovery, manage routing, and stream events.

- [Blueprint](https://github.com/steady-bytes/draft/tree/main/services/core/blueprint)- A distributed key/value store and service registry for handling service discovery.
- [Fuse](https://github.com/steady-bytes/draft/tree/main/services/core/fuse) - A [control plane](https://en.wikipedia.org/wiki/Control_plane) handling routing configuration to services running.
- [Catalyst](https://github.com/steady-bytes/draft/tree/main/services/core/catalyst) - An event streaming interface for real-time message production and consumption.
- [Beacon](/docs/architecture/beacon-observability) - An [OpenTelemetry](https://opentelemetry.io/docs/)-native observability service handling log, trace, and metric ingestion and query, backed by [ClickHouse](https://github.com/ClickHouse/ClickHouse). *(planned — see the linked design doc)*

Additionally, [envoy](https://www.envoyproxy.io/) is used as the ingress controller to the system. Envoy is controlled by routing rules configured through [Fuse](#fuse).

See below for a simple visual of a Draft cluster.

</br>
<img src="/images/docs/draft-system.png" alt="draft system diagram overview" style="border-radius: 1%; display: block; margin-left: auto; margin-right: auto; width: 80%;"/>
</br>

## Blueprint

Blueprint is a distributed key/value store and service registry. Blueprint is built to handle heavy write workloads while also maintaining data consistency between all nodes in the cluster.

Blueprint uses the [Raft Consensus Algorithm](https://raft.github.io/) to establish a leader of the cluster and to maintain quorum between the cluster nodes so that it can gracefully handle node failure and network partitions. If the leader is lost to the cluster a new election begins and another leader is agreed upon. For simplicity, only one Blueprint node (the leader) can accept a write request and write it to the LSM tree. If a follower node receives a write request then it automatically forwards the request to the leader and informs the client of the leader's connection details.

Blueprint also uses a combination of an [LSM tree](https://en.wikipedia.org/wiki/Log-structured_merge-tree) and a [FSM](https://en.wikipedia.org/wiki/Finite-state_machine) to efficiently store data on the file system and guarantee each object will be written to all nodes of the cluster correctly. This design gives any dependent system an efficient and reliable key/value storage layer.

Blueprint utilizes its own internal key/value store to provide the role of service registry, health, and wellness checks of registered Draft processes. You can think of a service registry as the system journal of a distributed system (consider [systemd](https://en.wikipedia.org/wiki/Systemd) as an example of this). Currently, Blueprint is not responsible for scheduling additional resources in case of failure (though we are considering this for the future) so its main responsibility is to keep track of the state and details of registered Draft [processes](https://github.com/steady-bytes/draft/blob/main/api/core/registry/service_discovery/v1/models.proto#L33). Two specific details are important and kept up to date.

- [Process Running State](https://github.com/steady-bytes/draft/blob/e7bb27cf10c60decbdb668cfbacf11ceadd9de86/api/core/registry/service_discovery/v1/models.proto#L88) - A value retained to determine if a process is ready for certain kinds of traffic.
- [Process Health State](https://github.com/steady-bytes/draft/blob/e7bb27cf10c60decbdb668cfbacf11ceadd9de86/api/core/registry/service_discovery/v1/models.proto#L96) - The healthiness state of the process.

### Process Registration

A Draft process must be provided with an _entrypoint_ into the cluster at boot. This entrypoint is the address of a [Blueprint](#blueprint) node within the cluster. At boot, the process will begin the registration flow which is outlined below:

1. A new Process sends a nonce to Blueprint
2. Blueprint will validate the nonce, create a _Process Identity_ and add the process to its system registry
3. The registering process will open a bi-directional RPC stream to the registry pushing it's _Running_ state and _Health_ state to the registry at a background polling rate
4. Blueprint will update its registry with any new details pushed over the above stream from the process and ack each message with the current Blueprint cluster details
5. When the process is ready to leave the cluster, it wo;; request to be removed from the registry and then close the bi-directional stream

See below for a diagram of the process registration flow:

</br>
<img src="/images/docs/draft-process-registration.png" alt="Blueprint process registration diagram" style="border-radius: 1%; display: block; margin-left: auto; margin-right: auto; width: 80%;"/>
</br>

{{< alert context="info" text="Once a process is registered, other services can use the metadata to lookup services they might want to communicate with. For example, the default behavior is to register the RPC service definitions of a process. So if process B needs to talk to process A via an RPC, then process B can establish a connection with A by looking it up in the registry." />}}

## Fuse

Fuse is the [control plane](https://en.wikipedia.org/wiki/Control_plane) for a draft cluster. When a service/process would like to expose routes to the outside world then a route can be registered with Fuse. Currently we only support [envy](https://www.envoyproxy.io/) as the ingress proxy, however we are considering supporting various proxies in the future. Below is how a route is established in the control plane.

1. Once a process has [registered](#process-registration) to Blueprint, the Fuse address is looked up and routing details are sent to Fuse.
2. Fuse will update envoy with the new route table
3. Optionally Fuse will store the routing information within Blueprint

</br>
<img src="/images/docs/fuse-route-registration.png" alt="Blueprint process registration diagram" style="border-radius: 1%; display: block; margin-left: auto; margin-right: auto; width: 80%;"/>
</br>

## Catalyst

Catalyst is the event interface of a Draft cluster. Catalyst can be run in two ways:

1. As a standalone message bus that will handle event processing for all consumers and producers.
2. As a pass-through interface for other message systems like [Apache Kafka](https://kafka.apache.org/), [Redpanda](https://www.redpanda.com/), and [NATS](https://nats.io/). We are in early development of this component. If your interested in on collaborating with use please [reach out](https://steady-bytes.com/contact).

### Known issues

**`Consume` fans events out to one subscriber at a time, not to every subscriber (found 2026-08-23, not yet fixed).** `services/core/catalyst/broker/atomicMap.go`'s `Broadcast` — despite its name — does not broadcast to every open `Consume` stream. Two things combine to cause this:

1. `Broadcast`'s (and the matching `Broker`/registration path's) routing key is computed as `hash(msg.ProtoReflect().Descriptor().FullName())` — the *Go struct's own proto type name*, i.e. always `"CloudEvent"`, regardless of the event's own `.Type` field (`"tooling.workflow.v1.RunStarted"`, `"examples.user.v1.UserCreated"`, etc.). Every producer and every consumer therefore shares exactly one routing key, cluster-wide, no matter what type they declared interest in via `ConsumeRequest.Message.Type`. (Client-side type filtering — every existing producer/consumer pair in this repo, e.g. `services/examples/consumer`, filters `event.Type` after receiving — is what makes this invisible under a *single* consumer of a given type.)
2. Delivery for that one shared key goes through one shared **unbuffered** `chan *acv1.CloudEvent` (`atomicMap.n[key]`), and every open `Consume` stream's `send` goroutine reads from that same channel. Go's unbuffered-channel semantics mean each `Broadcast(key, msg)` call (a single `ch <- msg`) is received by exactly **one** of the waiting goroutines, chosen essentially at random — not all of them.

Net effect: with more than one `Consume` stream open at the same time (even for different declared event types — see point 1), each produced event is delivered to exactly one of them, not to every interested subscriber. A single consumer works correctly (verified live: `services/tooling/bench` publishing `RunStarted`/`StepCompleted`/`RunFinished` events, received in full and in order by one subscriber). This was found while verifying Bench's own event publishing ([Bench — Workflow Engine](/docs/architecture/bench-workflow-engine)'s Phase 7) — Bench's producer-side code is correct and unaffected; this is pre-existing Catalyst broker behavior, not something Bench-side work can fix. **Not fixed as of this writing** — a fix needs `Broadcast`/`Broker` keyed on the event's own `.Type` (not the CloudEvent envelope's type) and, for true fan-out, delivering into a per-consumer copy of the message rather than one shared channel serving every stream.

## Beacon

*Planned — not yet implemented. See [Beacon — Observability](/docs/architecture/beacon-observability) for the full system design and phased implementation plan.*

Beacon is Draft's observability service: log, trace, and metric ingestion over standard [OTLP](https://opentelemetry.io/docs/specs/otlp/), backed by ClickHouse, with a Dioxus web client served the same way Blueprint serves its own UI. It is deliberately independent of Catalyst — telemetry volume and shape don't fit the CloudEvent bus, and Catalyst's own event bus has an [open fan-out bug](#known-issues) that rules out depending on it for ingestion. Beacon registers with Blueprint and routes through Fuse like every other core service.
