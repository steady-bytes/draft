package infra

import (
	"context"
	"time"

	"github.com/steady-bytes/tools/dctl/docker"
	"github.com/steady-bytes/tools/dctl/output"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/spf13/cobra"
)

var (
	Follow bool
)

const (
	mongoImage        = "mongo:7.0.2"
	mongoContainer    = "draft-mongo"
	postgresImage     = "postgres:16.0"
	postgresContainer = "draft-postgres"
	hasuraImage       = "hasura/graphql-engine:v2.34.0"
	hasuraContainer   = "draft-hasura"
	natsImage         = "nats:2.10.2"
	natsContainer     = "draft-nats"
)

func Start(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	dctl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	var config *container.Config
	var hostConfig *container.HostConfig

	// postgres
	config = &container.Config{
		Image: postgresImage,
		Env: []string{
			"POSTGRES_PASSWORD=admin",
			"POSTGRES_USER=admin",
			"POSTGRES_DB=postgres",
		},
		ExposedPorts: map[nat.Port]struct{}{
			"5432/tcp": {}},
	}
	hostConfig = &container.HostConfig{
		PortBindings: nat.PortMap{
			"5432/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: "5432",
				},
			},
		},
	}
	id, err := dctl.StartContainer(ctx, postgresContainer, config, hostConfig, Follow)
	if err != nil {
		output.Error(err)
		return err
	}
	if Follow {
		defer stop(context.Background(), dctl, id)
	}

	// mongo
	config = &container.Config{
		Image: mongoImage,
		Env: []string{
			"MONGO_INITDB_ROOT_USERNAME=admin",
			"MONGO_INITDB_ROOT_PASSWORD=admin",
		},
		ExposedPorts: map[nat.Port]struct{}{
			"27017/tcp": {}},
	}
	hostConfig = &container.HostConfig{
		PortBindings: nat.PortMap{
			"27017/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: "27017",
				},
			},
		},
	}
	id, err = dctl.StartContainer(ctx, mongoContainer, config, hostConfig, Follow)
	if err != nil {
		output.Error(err)
		return err
	}
	if Follow {
		defer stop(context.Background(), dctl, id)
	}

	// nats
	config = &container.Config{
		Image: natsImage,
		Env:   []string{},
		ExposedPorts: map[nat.Port]struct{}{
			"4222/tcp": {},
		},
	}
	hostConfig = &container.HostConfig{
		PortBindings: nat.PortMap{
			"4222/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: "4222",
				},
			},
		},
	}
	id, err = dctl.StartContainer(ctx, natsContainer, config, hostConfig, Follow)
	if err != nil {
		output.Error(err)
		return err
	}
	if Follow {
		defer stop(context.Background(), dctl, id)
	}

	// wait for databases to start before starting hasura
	time.Sleep(5 * time.Second)

	// hasura
	config = &container.Config{
		Image: hasuraImage,
		Env: []string{
			"HASURA_GRAPHQL_METADATA_DATABASE_URL=postgres://admin:admin@host.docker.internal:5432/postgres",
			"HASURA_GRAPHQL_DATABASE_URL=postgres://admin:admin@host.docker.internal:5432/postgres",
			"HASURA_GRAPHQL_ENABLE_CONSOLE=true",
			"HASURA_GRAPHQL_ENABLED_LOG_TYPES=startup, http-log, webhook-log, websocket-log, query-log",
			"HASURA_GRAPHQL_ENABLE_TELEMETRY=false",
		},
		ExposedPorts: map[nat.Port]struct{}{
			"8080/tcp": {},
		},
	}
	hostConfig = &container.HostConfig{
		PortBindings: nat.PortMap{
			"8080/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: "8082",
				},
			},
		},
	}
	id, err = dctl.StartContainer(ctx, hasuraContainer, config, hostConfig, Follow)
	if err != nil {
		output.Error(err)
		return err
	}
	if Follow {
		defer stop(context.Background(), dctl, id)
	}

	if Follow {
		<-ctx.Done()
	}

	return nil
}

func stop(ctx context.Context, dctl docker.DockerController, id string) {
	err := dctl.StopContainer(ctx, id)
	if err != nil {
		output.Error(err)
	}
}
