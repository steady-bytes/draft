# Fuse
A programmable `control plane` for the `envoy` proxy.

# Setup
This uses the default xds configuration. Envoy will be configured to connect to port `:18000` to get it's routing information.
```sh
envoy -c envoy-xds-config.yaml --drain-time-s 1 -l debug
```