package service_discovery

import (
	"context"
	"errors"
	"time"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	kv "github.com/steady-bytes/draft/services/core/blueprint/key_value"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"
)

const (
	// TODO: this was removed from the chassis and I'm not sure where it should go tbh
	GlobalNonceKey = "GLOBAL_NONCE"

	// staleThreshold is how long without a heartbeat before a process is considered dead.
	// Set to 3x the chassis sync interval to allow for a couple of missed heartbeats.
	staleThreshold = 3 * chassis.SYNC_INTERVAL

	// ReapInterval is how often the reaper loop runs. Exported so main.go can drive the ticker.
	ReapInterval = 30 * time.Second
)

type (
	Controller interface {
		ServiceDiscovery
	}

	ServiceDiscovery interface {
		Finalize(ctx context.Context, log chassis.Logger, pid string) error
		Initialize(ctx context.Context, log chassis.Logger, nonce, name string) (*sdv1.ProcessIdentity, error)
		Synchronize(ctx context.Context, log chassis.Logger, details *sdv1.ClientDetails)

		Query(ctx context.Context, log chassis.Logger) (map[string]*sdv1.Process, error)
		Reap(ctx context.Context, log chassis.Logger)

		Subscribe() (string, <-chan *ProcessEvent)
		Unsubscribe(id string)

		GetClusterDetails() *sdv1.ClusterDetails
		GetClusterLeaderAddress(logger chassis.Logger) (string, error)
	}

	controller struct {
		kvController   kv.Controller
		raftController chassis.RaftController
		secretStore    chassis.SecretStore
		broadcaster    *Broadcaster
	}
)

func NewController(kvController kv.Controller, raftController chassis.RaftController) Controller {
	return &controller{
		kvController:   kvController,
		raftController: raftController,
		secretStore:    nil,
		broadcaster:    NewBroadcaster(),
	}
}

func (c *controller) Subscribe() (string, <-chan *ProcessEvent) {
	return c.broadcaster.Subscribe()
}

func (c *controller) Unsubscribe(id string) {
	c.broadcaster.Unsubscribe(id)
}

// Accepts a `SecretStore` interface and adds it to the controller
func (c *controller) SetSecretStore(s chassis.SecretStore) {
	c.secretStore = s
}

const (
	signKey                           = "TODO -> load this from the secret store"
	ErrFailedNonce                    = "nonce failure"
	ErrFailedProcessAlreadyRegistered = "process has already be initialized"
	ErrFailedToMarshalPayload         = "failed to marshal payload"
	ErrFailedToSaveProcessDetails     = "failed to save process details"
	ErrFailedToGetProcessDetails      = "failed to lookup process details"
	ErrFailedTokenForge               = "failed to forge the token"
	ErrFailedTypeCast                 = "failed to cast type"
)

// Initialize - When a service starts and wants to register itself with the system then a unique name, and system nonce
// can be provided to get `ProcessIdentity` details so that A process can then finalize service registration
func (c *controller) Initialize(ctx context.Context, log chassis.Logger, nonce, name string) (*sdv1.ProcessIdentity, error) {
	var (
		err     error
		process *sdv1.Process
		pAny    = &anypb.Any{}
	)

	// validate the nonce (this will also require that a nonce is read in by the chassis).
	// TODO (@andrewsc208): Make a default `SecretStore` that will use the existing `key_value` store as it's persistence layer.
	//                      Long term integrations with Vault, or other secret stores can be added later. This will allow for
	//					    an enterprise to bring their own store.
	// n, err := c.secretStore.Get(ctx, GlobalNonceKey)
	// if err != nil || n != nonce {
	// 	return nil, errors.New(ErrFailedNonce)
	// }

	// reuse the PID of a disconnected process with the same name if one exists
	existing, err := c.Query(ctx, log)
	if err == nil {
		for _, p := range existing {
			if p.Name == name && p.RunningState == sdv1.ProcessRunningState_PROCESS_DICONNECTED {
				process = p
				break
			}
		}
	}

	if process == nil {
		process = &sdv1.Process{
			Pid:         uuid.NewString(),
			Name:        name,
			ProcessKind: sdv1.ProcessKind_SERVER_PROCESS,
			Metadata:    []*sdv1.Metadata{},
			JoinedTime:  timestamppb.Now(),
		}
	}

	process.RunningState = sdv1.ProcessRunningState_PROCESS_STARTING
	process.HealthState = sdv1.ProcessHealthState_PROCESS_HEALTHY

	// generate a fresh token for this connection
	process.Token, err = c.forgeIdentityToken()
	if err != nil {
		return nil, errors.New(ErrFailedTokenForge)
	}

	pAny, err = anypb.New(process)
	if err != nil {
		return nil, errors.New(ErrFailedTypeCast)
	}

	_, err = c.kvController.Set(log, process.Pid, pAny, 500*time.Millisecond)
	if err != nil {
		return nil, errors.New(ErrFailedToSaveProcessDetails)
	}

	c.broadcaster.Publish(process)

	// TODO (@andrewsc208): Get the leaders registry address to send synchronize packets to

	return &sdv1.ProcessIdentity{
		Pid:             process.Pid,
		RegistryAddress: "localhost:2221",
		Token:           process.Token,
	}, nil
}

// Synchronize - receive a message from an `Initialized` process and update it's state in the `SystemJournal`.
func (c *controller) Synchronize(ctx context.Context, log chassis.Logger, details *sdv1.ClientDetails) {
	var (
		err     error
		process = &sdv1.Process{}
		pAny    = &anypb.Any{}
	)

	pAny, err = anypb.New(process)
	if err != nil {
		log.WithError(kv.ErrFailedAnyCast)
		return
	}

	// check that process has already been added to the `SystemJournal`
	pAny, err = c.kvController.Get(log, details.Pid, pAny)
	if err != nil {
		log.WithError(err)
		return
	}

	if pAny.MessageIs(process) {
		if err := anypb.UnmarshalTo(pAny, process, proto.UnmarshalOptions{}); err != nil {
			log.WithError(err)
			return
		}
	}

	// ignore if the wrong token is sent
	if process.Token.GetJwt() != details.Token {
		return
	}

	process.HealthState = details.HealthState
	process.Location = details.Location
	process.Metadata = details.Metadata
	process.ProcessKind = details.ProcessKind
	process.RunningState = details.RunningState
	process.LastStatusTime = timestamppb.Now()
	process.Metadata = details.Metadata
	process.IpAddress = details.AdvertiseAddress

	pAny, err = anypb.New(process)
	if err != nil {
		log.WithError(kv.ErrFailedAnyCast)
		return
	}

	_, err = c.kvController.Set(log, process.Pid, pAny, 500*time.Millisecond)
	if err != nil {
		log.Error(ErrFailedToSaveProcessDetails)
		return
	}

	c.broadcaster.Publish(process)
}

// Finalize - Mark the process as disconnected and unhealthy in the registry.
func (c *controller) Finalize(ctx context.Context, log chassis.Logger, pid string) error {
	var (
		err     error
		process = &sdv1.Process{}
		pAny    = &anypb.Any{}
	)

	pAny, err = anypb.New(process)
	if err != nil {
		log.Error(ErrFailedTypeCast)
		return errors.New(ErrFailedTypeCast)
	}

	pAny, err = c.kvController.Get(log, pid, pAny)
	if err != nil {
		log.WithError(err)
		return err
	}

	if err = anypb.UnmarshalTo(pAny, process, proto.UnmarshalOptions{}); err != nil {
		log.WithError(err)
		return err
	}

	process.RunningState = sdv1.ProcessRunningState_PROCESS_DICONNECTED
	process.HealthState = sdv1.ProcessHealthState_PROCESS_UNHEALTHY

	pAny, err = anypb.New(process)
	if err != nil {
		log.WithError(err)
		return errors.New(ErrFailedTypeCast)
	}

	if _, err = c.kvController.Set(log, pid, pAny, 500*time.Millisecond); err != nil {
		log.WithError(err)
		return err
	}

	c.broadcaster.Publish(process)

	return nil
}

func (c *controller) Reap(ctx context.Context, log chassis.Logger) {
	if c.raftController.Stats(ctx)["state"] != "Leader" {
		return
	}

	processes, err := c.Query(ctx, log)
	if err != nil {
		log.WithError(err).Error("reaper: failed to query processes")
		return
	}

	for _, process := range processes {
		if process.LastStatusTime == nil {
			continue
		}
		if time.Since(process.LastStatusTime.AsTime()) > staleThreshold {
			log.WithField("pid", process.Pid).WithField("name", process.Name).Warn("reaper: marking stale process as disconnected")

			process.RunningState = sdv1.ProcessRunningState_PROCESS_DICONNECTED
			process.HealthState = sdv1.ProcessHealthState_PROCESS_UNHEALTHY

			pAny, err := anypb.New(process)
			if err != nil {
				log.WithError(err).WithField("pid", process.Pid).Error("reaper: failed to marshal process")
				continue
			}

			if _, err := c.kvController.Set(log, process.Pid, pAny, 500*time.Millisecond); err != nil {
				log.WithError(err).WithField("pid", process.Pid).Error("reaper: failed to update stale process")
				continue
			}

			c.broadcaster.Publish(process)
		}
	}
}

func (c *controller) Query(ctx context.Context, log chassis.Logger) (map[string]*sdv1.Process, error) {
	log.Trace("query")

	var (
		err           error
		process       = &sdv1.Process{}
		pAny          = &anypb.Any{}
		systemJournal = map[string]*sdv1.Process{}
	)

	pAny, err = anypb.New(process)
	if err != nil {
		log.Error(ErrFailedTypeCast)
		// return nil, errors.New(ErrFailedTypeCast)
	}

	res, err := c.kvController.List(log, pAny)
	if err != nil {
		log.WithError(err).Error(ErrFailedToGetProcessDetails)
		// return nil, errors.New(ErrFailedToGetProcessDetails)
	}

	// convert map from map[string]*anypb.Any to map[string]*sdv1.Process
	for _, v := range res {
		if v.MessageIs(process) {
			p := &sdv1.Process{}
			if err := anypb.UnmarshalTo(v, p, proto.UnmarshalOptions{}); err != nil {
				log.WithError(err)
			}
			systemJournal[p.Pid] = p
		}
	}

	return systemJournal, nil
}

func (c *controller) GetClusterDetails() *sdv1.ClusterDetails {
	cluster := c.raftController.GetClusterDetails()

	cd := &sdv1.ClusterDetails{
		Nodes: []*sdv1.Node{},
	}
	for _, v := range cluster.Servers {
		cd.Nodes = append(cd.Nodes, &sdv1.Node{
			Id:               string(v.ID),
			Address:          string(v.Address),
			LeadershipStatus: 0,
		})
	}

	return cd
}

func (c *controller) GetClusterLeaderAddress(logger chassis.Logger) (string, error) {
	a, _ := anypb.New(&kvv1.Value{})
	anyValue, err := c.kvController.Get(logger, "leader", a)
	if err != nil {
		logger.WithError(err).Error("failed to get leader address")
		return "", err
	}

	v := &kvv1.Value{}
	err = anypb.UnmarshalTo(anyValue, v, proto.UnmarshalOptions{})
	if err != nil {
		logger.WithError(err).Error("failed to unmarshal leader value")
		return "", err
	}

	return v.Data, nil
}

// TODO -> Figure out how I want to generate a token for the process
func (c *controller) forgeIdentityToken() (*sdv1.Token, error) {
	// t := jwt.New(jwt.GetSigningMethod("RS256"))
	// return t.SignedString(signKey)

	return &sdv1.Token{
		Id:  uuid.NewString(),
		Jwt: "test",
	}, nil
}
