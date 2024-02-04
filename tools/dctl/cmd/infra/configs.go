package infra

import (
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

type infraConfig struct {
	containerConfig *container.Config
	hostConfig      *container.HostConfig
}

var (
	infraConfigs = map[string]infraConfig{
		"mongo": {
			containerConfig: &container.Config{
				Image: "mongo:7",
				Env: []string{
					"MONGO_INITDB_ROOT_USERNAME=admin",
					"MONGO_INITDB_ROOT_PASSWORD=admin",
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
					"POSTGRES_PASSWORD=postgres",
					"POSTGRES_USER=postgres",
					"POSTGRES_DB=postgres",
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
		// NOTE: keep hasura as the last configuration so it is started after all databases it might depend on
		"hasura": {
			containerConfig: &container.Config{
				Image: "hasura/graphql-engine:v2.37.0",
				Env: []string{
					"HASURA_GRAPHQL_METADATA_DATABASE_URL=postgres://postgres:postgres@host.docker.internal:5432/postgres",
					"HASURA_GRAPHQL_DATABASE_URL=postgres://postgres:postgres@host.docker.internal:5432/postgres",
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
