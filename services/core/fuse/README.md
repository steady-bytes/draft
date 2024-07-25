# Fuse
A programmable `control plane` for the `envoy` proxy.

# Setup
1. [Install Envoy Proxy](https://www.envoyproxy.io/docs/envoy/latest/start/install)
__NOTE:__ It's assumed that `envoy` will be available on your path.

2. Start `blueprint` and `fuse` with the configuration file found in the tests directory (`tests/fuse/config.yaml`)

3. Start `envoy` with the default dynamic `xds` configuration. Envoy will be configured to connect to port `:18000` to get it's routing information.
```sh
envoy -c envoy-xds-config.yaml --drain-time-s 1 -l debug
```

Dynamic routing is now setup. 