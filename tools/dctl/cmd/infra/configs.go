package infra

import (
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/docker/go-units"
)

type infraConfig struct {
	containerConfig *container.Config
	hostConfig      *container.HostConfig
}

var (
	infraConfigs = map[string]infraConfig{
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
					"HASURA_GRAPHQL_METADATA_DATABASE_URL=postgres://draft:draft@host.docker.internal:5432/draft",
					"HASURA_GRAPHQL_DATABASE_URL=postgres://draft:draft@host.docker.internal:5432/draft",
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
							HostPort: "8082",
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
