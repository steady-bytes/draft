# Fuse

A programmable `Control Plane` for the `envoy` proxy. `Fuse` is design to be the central component of the system responsible for controlling `layer 4` and up network routing in you the system. This could be direct `UDP` traffic or application level `gRPC` service calls routed to and from an external client. Distributed system are unreliable it's often a the case services may fail, or networks will partition. Being able to dynamically control routing decisions can help in building a more reliable distributed system. 

# Features
* Fully dynamic routing rules with zero network downtime
* namespace, and domain based routing
* OIDC integration
* Default `Draft` chassis support (Use the chassis, and get the free integration)
* gRPC support

## Getting Started

To get started you will need `envoy` installed on your local system and executable from the command line. Optionally, if you would like to persist configuration saved to `fuse` run an instance of `blueprint`. Once your env is prepared run `startup.sh`. This will run `envoy` and tell it that `fuse` is running on `:18000`.

### Process Integration
Since the `control plane` is a hidden feature of most systems. It's good to know how exactly the integration with application services will work. `Draft` as a whole is a framework for building reliable distributed systems. Unpacking a little, the system is going to have processes receiving packets on different ports. We need to allow the outside clients/users access resources. To do this really well we need to control routing details as the system changes overtime. Service A starts up and exposes some new feature that we want to test on 30% of our customers. This is where `fuse` and `envoy` come in. `Envoy` is a capable network proxy, and `fuse` allows for an integration point between application processes and network routing rules.

The standard integration that is currently supported is as follows:
1. Application Process Starts
2. Reaches out to `blueprint` to get stored configuration for `fuse`
3. Once the service is ready to start serving traffic
4. Update routing information directly to `fuse`

### Design Overview
Now that we have a general understanding of what `fuse` is responsible for. Let's break down the design. Again `envoy` is our ingress proxy. It's what will be exposed to the outside world on `:80` and `:443`. You tell `envoy` to connect to `fuse` and when a service comes online and needs to route traffic. It tells `fuse` what to do. Simple as that.

[INSERT DIAGRAM SCREEN SHOT]
