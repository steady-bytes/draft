package infra

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	"github.com/docker/go-units"
)

type (
	infraConfig struct {
		containerConfig *container.Config
		hostConfig      *container.HostConfig
		configFile      *configFile
		mountPoint      *mountPoint
	}
	configFile struct {
		mountPath string
		contents  string
	}
	mountPoint struct {
		mountPath string
	}
)

var (
	infraConfigs = map[string]infraConfig{
		"blueprint": {
			containerConfig: &container.Config{
				Image: "ghcr.io/steady-bytes/draft-core-blueprint:latest",
				ExposedPorts: map[nat.Port]struct{}{
					"2221/tcp": {},
					"1111/tcp": {},
				},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"2221/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "2221",
						},
					},
					"1111/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "1111",
						},
					},
				},
			},
			configFile: &configFile{
				mountPath: "/etc/config.yaml",
				contents: `
service:
  name: blueprint
  domain: core

  logging:
    level: info

  network:
    bind_address: 0.0.0.0
    bind_port: 2221


badger:
  path: /etc/badger

raft:
  node-id: node_1
  address: localhost
  port: 1111
  bootstrap: true
`,
			},
			mountPoint: &mountPoint{
				mountPath: "/etc/badger",
			},
		},
		"catalyst": {
			containerConfig: &container.Config{
				Image: "ghcr.io/steady-bytes/draft-core-catalyst:latest",
				ExposedPorts: map[nat.Port]struct{}{
					"2220/tcp": {},
				},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"2220/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "2220",
						},
					},
				},
			},
			configFile: &configFile{
				mountPath: "/etc/config.yaml",
				contents: `
service:
  name: catalyst
  domain: core
  entrypoint: http://draft-blueprint:2221

  logging:
    level: info

  network:
    bind_address: 0.0.0.0
    bind_port: 2220
    internal:
      host: localhost
      port: 2220
`,
			},
		},
		"fuse": {
			containerConfig: &container.Config{
				Image: "ghcr.io/steady-bytes/draft-core-fuse:latest",
				ExposedPorts: map[nat.Port]struct{}{
					"18000/tcp": {},
				},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"18000/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "18000",
						},
					},
				},
			},
			configFile: &configFile{
				mountPath: "/etc/config.yaml",
				contents: `
service:
  name: fuse
  domain: core
  entrypoint: http://draft-blueprint:2221

  logging:
    level: info

  network:
    bind_address: 0.0.0.0
    bind_port: 18000
    internal:
      host: localhost
      port: 18000

fuse:
  address: http://localhost:18000
  listener:
    address: 0.0.0.0
    port: 10000
`,
			},
		},
		"envoy": {
			containerConfig: &container.Config{
				Image: "envoyproxy/envoy:v1.31.2",
				ExposedPorts: map[nat.Port]struct{}{
					"10000/tcp": {},
					"18090/tcp": {},
				},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"10000/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "10000",
						},
					},
					"19000/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "19000",
						},
					},
					"18090/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "18090",
						},
					},
				},
			},
			configFile: &configFile{
				mountPath: "/etc/envoy/envoy.yaml",
				contents: `
node:
  cluster: fuse-proxy
  id: fuse-proxy-1

admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 19000

dynamic_resources:
  cds_config:
    resource_api_version: V3
    api_config_source:
      api_type: GRPC
      transport_api_version: V3
      grpc_services:
        - envoy_grpc:
            cluster_name: xds_cluster
      set_node_on_first_message_only: true
  lds_config:
    resource_api_version: V3
    api_config_source:
      api_type: GRPC
      transport_api_version: V3
      grpc_services:
        - envoy_grpc:
            cluster_name: xds_cluster
      set_node_on_first_message_only: true

static_resources:
  clusters:
    - name: xds_cluster
      connect_timeout: 1s
      load_assignment:
        cluster_name: xds_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      # address in which fuse is running on
                      address: draft-fuse
                      port_value: 18000
      http2_protocol_options: {}
      type: STRICT_DNS
    - name: als_cluster
      connect_timeout: 1s
      load_assignment:
        cluster_name: als_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: 0.0.0.0
                      port_value: 18090
      http2_protocol_options: {}

layered_runtime:
  layers:
    - name: runtime-0
      rtds_layer:
        rtds_config:
          resource_api_version: V3
          api_config_source:
            transport_api_version: V3
            api_type: GRPC
            grpc_services:
              envoy_grpc:
                cluster_name: xds_cluster
        name: runtime-0
`,
			},
		},
		"clickhouse": {
			containerConfig: &container.Config{
				Image: "clickhouse/clickhouse-server:23-alpine",
				ExposedPorts: map[nat.Port]struct{}{
					"9000/tcp": {},
				},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"9000/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "9000",
						},
					},
				},
				Resources: container.Resources{
					Ulimits: []*units.Ulimit{
						{
							Name: "nofile",
							Hard: 262144,
							Soft: 262144,
						},
					},
				},
			},
		},
		"mongo": {
			containerConfig: &container.Config{
				Image: "mongo:7",
				Env: []string{
					"MONGO_INITDB_ROOT_USERNAME=draft",
					"MONGO_INITDB_ROOT_PASSWORD=draft",
				},
				ExposedPorts: map[nat.Port]struct{}{
					"27017/tcp": {}},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"27017/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "27017",
						},
					},
				},
			},
		},
		"nats": {
			containerConfig: &container.Config{
				Image: "nats:2",
				Env:   []string{},
				ExposedPorts: map[nat.Port]struct{}{
					"4222/tcp": {},
				},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"4222/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "4222",
						},
					},
				},
			},
		},
		"postgres": {
			containerConfig: &container.Config{
				Image: "postgres:16",
				Env: []string{
					"POSTGRES_PASSWORD=draft",
					"POSTGRES_USER=draft",
					"POSTGRES_DB=draft",
				},
				ExposedPorts: map[nat.Port]struct{}{
					"5432/tcp": {}},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"5432/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "5432",
						},
					},
				},
			},
		},
		"rabbitmq": {
			containerConfig: &container.Config{
				Image: "rabbitmq:3",
				Env: []string{
					"RABBITMQ_DEFAULT_USER=draft",
					"RABBITMQ_DEFAULT_PASS=draft",
				},
				ExposedPorts: map[nat.Port]struct{}{
					"5672/tcp":  {},
					"15672/tcp": {},
				},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"5672/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "5672",
						},
					},
					"15672/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "15672",
						},
					},
				},
			},
		},
		"vault": {
			containerConfig: &container.Config{
				Image: "vault:1.13.3",
				Env: []string{
					"VAULT_DEV_ROOT_TOKEN_ID=myroot",
				},
				ExposedPorts: map[nat.Port]struct{}{
					"8200/tcp": {},
				},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"8200/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "8200",
						},
					},
				},
				CapAdd: []string{
					"IPC_LOCK",
				},
			},
		},

		// NOTE: keep hasura as the last configuration so it is started after all databases it might depend on
		"hasura": {
			containerConfig: &container.Config{
				Image: "hasura/graphql-engine:v2.37.0",
				Env: []string{
					"HASURA_GRAPHQL_METADATA_DATABASE_URL=postgres://draft:draft@draft-postgres:5432/draft",
					"HASURA_GRAPHQL_DATABASE_URL=postgres://draft:draft@draft-postgres:5432/draft",
					"HASURA_GRAPHQL_ENABLE_CONSOLE=true",
					"HASURA_GRAPHQL_ENABLED_LOG_TYPES=startup, http-log, webhook-log, websocket-log, query-log",
					"HASURA_GRAPHQL_ENABLE_TELEMETRY=false",
				},
				ExposedPorts: map[nat.Port]struct{}{
					"8080/tcp": {},
				},
			},
			hostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"8080/tcp": []nat.PortBinding{
						{
							HostIP:   "0.0.0.0",
							HostPort: "8080",
						},
					},
				},
			},
		},
	}
)

func containerName(infra string) string {
	return fmt.Sprintf("draft-%s", infra)
}

func getInfraConfig(name string) (infraConfig, error) {
	config, ok := infraConfigs[name]
	if !ok {
		return config, fmt.Errorf("invalid infra service name: %s", name)
	}

	if config.configFile != nil {
		path, err := configPath()
		if err != nil {
			return config, err
		}
		config.hostConfig.Binds = []string{
			fmt.Sprintf("%s:%s", filepath.Join(path, fmt.Sprintf("%s.yaml", name)), config.configFile.mountPath),
		}
	}

	if config.mountPoint != nil {
		path, err := configPath()
		if err != nil {
			return config, err
		}
		config.hostConfig.Mounts = []mount.Mount{
			{
				Type: mount.TypeBind,
				Source: filepath.Join(path, name),
				Target: config.mountPoint.mountPath,
			},
		}
	}

	return config, nil
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dctl", "infra"), nil
}
