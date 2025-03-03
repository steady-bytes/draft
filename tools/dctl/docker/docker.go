package docker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/google/uuid"
	"github.com/moby/term"
)

const (
	errContainerNotFound = "container not found"
)

type (
	DockerController interface {
		ImageController
		ContainerController
		NetworkController
	}
	ImageController interface {
		// BuildImage takes a file path and an image name and builds a Docker image using the Dockerfile in the given directory
		BuildImage(ctx context.Context, path, image string) error
		// PullImage pulls the given image down from the Docker registry
		PullImage(ctx context.Context, image string) error
		// TagImage tags an existing image with a new tag
		TagImage(ctx context.Context, image string, source, target string) error
		// PushImage pushes the given image up to the Docker registry
		PushImage(ctx context.Context, image string) error
	}
	ContainerController interface {
		// RunContainer creates a container, starts it, waits for it to complete, and removes it if requested
		RunContainer(ctx context.Context, containerName string, config *container.Config, host *container.HostConfig, showOutput bool) error
		// StartContainer runs an existing container or creates a new one if none already exist with the given name. It exists without waiting for the container to exit
		StartContainer(ctx context.Context, containerName string, config *container.Config, host *container.HostConfig, showOutput bool) (string, error)
		// StopContainer stops a container by the given id
		StopContainer(ctx context.Context, id string) error
		// StopContainerByName stops a container by the given name
		StopContainerByName(ctx context.Context, containerName string) error
		// RemoveContainer removes a container by the given id
		RemoveContainer(ctx context.Context, id string) error
		// RemoveContainerByName removes a container by the given name
		RemoveContainerByName(ctx context.Context, containerName string) error
		// GetContainerByName gets a container by the given name
		GetContainerByName(ctx context.Context, containerName string) (*types.Container, error)
		// GenerateContainerName generates a random container name in the format of: dctl-UUID
		GenerateContainerName() (name string)
	}
	NetworkController interface {
		// CreateNetwork creates a network with the given name
		CreateNetwork(ctx context.Context, name string) error
		// RemoveNetwork removes the network with the given name
		RemoveNetwork(ctx context.Context, name string) error
		// GetNetworkByName gets a network by the given name
		// GetNetworkByName(ctx context.Context, name string) (*types.NetworkResource, error)
	}
)

type dockerController struct {
	cli *client.Client
}

func NewDockerController() (DockerController, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &dockerController{
		cli: cli,
	}, nil
}

func (d *dockerController) BuildImage(ctx context.Context, path, image string) error {
	buildContext, err := d.getDockerContext(path)
	if err != nil {
		return err
	}
	opt := types.ImageBuildOptions{
		Tags: []string{image},
	}
	resp, err := d.cli.ImageBuild(ctx, buildContext, opt)
	if err != nil {
		return err
	}

	id, isTerm := term.GetFdInfo(os.Stdout)
	_ = jsonmessage.DisplayJSONMessagesStream(resp.Body, os.Stdout, id, isTerm, nil)

	return nil
}

func (d *dockerController) PullImage(ctx context.Context, image string) error {
	output.Print("Pulling image: %s", image)
	resp, err := d.cli.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return err
	}

	id, isTerm := term.GetFdInfo(os.Stdout)
	_ = jsonmessage.DisplayJSONMessagesStream(resp, os.Stdout, id, isTerm, nil)

	return nil
}

func (d *dockerController) TagImage(ctx context.Context, image string, source, target string) error {
	output.Print("Tagging image %s:%s with new tag %s", image, source, target)
	s := fmt.Sprintf("%s:%s", image, source)
	t := fmt.Sprintf("%s:%s", image, target)
	err := d.cli.ImageTag(ctx, s, t)
	if err != nil {
		return nil
	}
	return nil
}

func (d *dockerController) PushImage(ctx context.Context, image string) error {
	output.Print("Pushing image: %s", image)

	resp, err := d.cli.ImagePush(ctx, image, types.ImagePushOptions{
		RegistryAuth: auth(),
	})
	if err != nil {
		return err
	}

	id, isTerm := term.GetFdInfo(os.Stdout)
	_ = jsonmessage.DisplayJSONMessagesStream(resp, os.Stdout, id, isTerm, nil)

	return nil
}

func (d *dockerController) RunContainer(ctx context.Context, containerName string, config *container.Config, host *container.HostConfig, showOutput bool) error {
	output.Print("Running container: %s", containerName)

	// configure and create container
	if showOutput {
		config.AttachStdout = true
		config.AttachStderr = true
		config.Tty = true
	}
	resp, err := d.cli.ContainerCreate(ctx, config, host, nil, nil, containerName)
	if err != nil {
		return err
	}

	// start container
	err = d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
	if err != nil {
		return err
	}

	if showOutput {
		// retrieve and print logs
		err := d.printContainerLogs(ctx, containerName)
		if err != nil {
			return err
		}
	}

	// wait for container to complete
	_, err = d.waitForContainer(ctx, resp.ID)
	if err != nil {
		return err
	}

	return nil
}

func (d *dockerController) StartContainer(ctx context.Context, containerName string, config *container.Config, host *container.HostConfig, showOutput bool) (string, error) {
	output.Print("Starting container: %s", containerName)

	id, err := d.getContainerID(ctx, containerName)
	if err != nil {
		return "", nil
	}

	if id == "" {
		// configure and create container
		if showOutput {
			config.AttachStdout = true
			config.AttachStderr = true
			config.Tty = true
		}
		resp, err := d.cli.ContainerCreate(ctx, config, host, nil, nil, containerName)
		if err != nil {
			return "", err
		}
		id = resp.ID

		// connect container to draft network network
		network, err := d.getNetworkByName(ctx, "draft")
		if err != nil {
			return "", err
		}

		err = d.cli.NetworkConnect(ctx, network.ID, id, nil)
		if err != nil {
			return "", err
		}
	}

	// start container
	err = d.cli.ContainerStart(ctx, id, container.StartOptions{})
	if err != nil {
		return "", err
	}

	if showOutput {
		// retrieve and print logs
		err := d.printContainerLogs(ctx, containerName)
		if err != nil {
			return "", err
		}
	}

	return id, nil
}

func (d *dockerController) StopContainer(ctx context.Context, id string) error {
	timeout := 30
	options := container.StopOptions{
		Timeout: &timeout,
	}
	return d.cli.ContainerStop(ctx, id, options)
}

func (d *dockerController) StopContainerByName(ctx context.Context, containerName string) error {
	output.Print("Stopping container: %s", containerName)
	id, err := d.getContainerID(ctx, containerName)
	if err != nil {
		return err
	}
	return d.StopContainer(ctx, id)
}

func (d *dockerController) RemoveContainer(ctx context.Context, id string) error {
	return d.cli.ContainerRemove(ctx, id, container.RemoveOptions{})
}

func (d *dockerController) RemoveContainerByName(ctx context.Context, containerName string) error {
	output.Print("Removing container: %s", containerName)
	id, err := d.getContainerID(ctx, containerName)
	if err != nil {
		return err
	}
	return d.RemoveContainer(ctx, id)
}

func (d *dockerController) GetContainerByName(ctx context.Context, containerName string) (*types.Container, error) {
	container, err := d.getContainerByName(ctx, containerName)
	if err != nil {
		if err.Error() != errContainerNotFound {
			return nil, err
		}
	}
	return container, nil
}

func (d *dockerController) GenerateContainerName() (name string) {
	return fmt.Sprintf("dctl-%s", uuid.New().String())
}

func (d *dockerController) CreateNetwork(ctx context.Context, name string) error {
	_, err := d.cli.NetworkCreate(ctx, name, types.NetworkCreate{})
	return err
}

func (d *dockerController) RemoveNetwork(ctx context.Context, name string) error {
	network, err := d.getNetworkByName(ctx, name)
	if err != nil {
		return err
	}
	return d.cli.NetworkRemove(ctx, network.ID)
}

// HELPER FUNCTIONS

func (d *dockerController) getDockerContext(filePath string) (io.Reader, error) {
	ctx, err := archive.TarWithOptions(filePath, &archive.TarOptions{})
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func (d *dockerController) waitForContainer(ctx context.Context, id string) (state int64, err error) {
	resultC, errC := d.cli.ContainerWait(ctx, id, "")
	select {
	case err := <-errC:
		return 0, err
	case result := <-resultC:
		return result.StatusCode, nil
	}
}

func (d *dockerController) printContainerLogs(ctx context.Context, containerName string) error {
	reader, err := d.cli.ContainerLogs(ctx, containerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Since:      "1s",
	})
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(reader)
	go func() {
		for scanner.Scan() {
			output.PrintlnWithNameAndColor(containerName, scanner.Text(), output.Blue)
		}
	}()

	return nil
}

func (d *dockerController) getContainerID(ctx context.Context, containerName string) (string, error) {
	id := ""
	exists := false
	list, err := d.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return id, err
	}
	for _, c := range list {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == containerName {
				exists = true
				id = c.ID
				break
			}
		}
		if exists {
			break
		}
	}
	return id, nil
}

func (d *dockerController) getContainerByName(ctx context.Context, containerName string) (*types.Container, error) {
	var con *types.Container
	exists := false
	list, err := d.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == containerName {
				exists = true
				con = &c
				break
			}
		}
		if exists {
			break
		}
	}
	if con == nil {
		return nil, fmt.Errorf(errContainerNotFound)
	}
	return con, nil
}

func (d *dockerController) getNetworkByName(ctx context.Context, name string) (*types.NetworkResource, error) {
	var net *types.NetworkResource

	list, err := d.cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}

	for _, network := range list {
		if network.Name == name {
			net = &network
			break
		}
	}

	if net == nil {
		return nil, fmt.Errorf("network not found")
	}

	return net, nil
}

func auth() string {
	authConfig := registry.AuthConfig{
		Username: os.Getenv("CONTAINER_REGISTRY_USERNAME"),
		Password: os.Getenv("CONTAINER_REGISTRY_PASSWORD"),
	}
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(encodedJSON)
}
