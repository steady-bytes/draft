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

	// DeregisterThreshold is how long a process may sit disconnected before the reaper deletes
	// its registry entry outright, mirroring Consul's DeregisterCriticalServiceAfter. A process
	// that restarts before this elapses never even reaches it — Initialize's deterministic
	// identity (see processID) upserts its existing row instead of leaving a disconnected one
	// behind. This tier exists for processes that are actually gone for good, and for cleaning up
	// entries left over from before deterministic identity existed.
	DeregisterThreshold = 5 * time.Minute
)

// processNamespaceUUID seeds the deterministic (UUIDv5) process identity computed by processID.
// It's an arbitrary, fixed UUID — it doesn't need to mean anything, only to stay stable across
// builds so the same (name, advertiseAddress) always hashes to the same id.
var processNamespaceUUID = uuid.MustParse("8f3b6c1a-8e34-4b7e-9b8a-2f6b7b9a4d10")

// processID derives a deterministic process identity from name and advertiseAddress, so the same
// logical instance (same name, same reachable address) always computes the same id across
// restarts. This makes Initialize a pure upsert instead of needing to race-detect and reuse a
// prior disconnected entry — see docs/architecture/service-registry-identity.md.
func processID(name, advertiseAddress string) string {
	return uuid.NewSHA1(processNamespaceUUID, []byte(name+"@"+advertiseAddress)).String()
}

type (
	Controller interface {
		ServiceDiscovery
	}

	ServiceDiscovery interface {
		Finalize(ctx context.Context, log chassis.Logger, pid string) error
		Initialize(ctx context.Context, log chassis.Logger, nonce, name, advertiseAddress string) (*sdv1.ProcessIdentity, error)
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
//
// The process identity is deterministic — a UUIDv5 derived from `name` + `advertiseAddress` (see
// processID) — rather than a fresh random UUID per call. The same logical instance (same name,
// same reachable address) always computes the same id, so Initialize is a pure upsert: a restart
// naturally overwrites its own prior registry row instead of racing to detect and reuse it. See
// docs/architecture/service-registry-identity.md for the full rationale.
func (c *controller) Initialize(ctx context.Context, log chassis.Logger, nonce, name, advertiseAddress string) (*sdv1.ProcessIdentity, error) {
	var (
		err  error
		pAny = &anypb.Any{}
	)

	// validate the nonce (this will also require that a nonce is read in by the chassis).
	// TODO (@andrewsc208): Make a default `SecretStore` that will use the existing `key_value` store as it's persistence layer.
	//                      Long term integrations with Vault, or other secret stores can be added later. This will allow for
	//					    an enterprise to bring their own store.
	// n, err := c.secretStore.Get(ctx, GlobalNonceKey)
	// if err != nil || n != nonce {
	// 	return nil, errors.New(ErrFailedNonce)
	// }

	process := &sdv1.Process{
		Pid:         processID(name, advertiseAddress),
		Name:        name,
		ProcessKind: sdv1.ProcessKind_SERVER_PROCESS,
		Metadata:    []*sdv1.Metadata{},
		JoinedTime:  timestamppb.Now(),
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
		sinceLastStatus := time.Since(process.LastStatusTime.AsTime())

		// Already disconnected and past DeregisterThreshold: remove the entry outright rather
		// than leaving a permanent tombstone. This also cleans up rows left over from before
		// deterministic identity existed, with no special-case migration needed.
		if process.RunningState == sdv1.ProcessRunningState_PROCESS_DICONNECTED && sinceLastStatus > DeregisterThreshold {
			log.WithField("pid", process.Pid).WithField("name", process.Name).Warn("reaper: deregistering process disconnected past threshold")

			pAny, err := anypb.New(process)
			if err != nil {
				log.WithError(err).WithField("pid", process.Pid).Error("reaper: failed to marshal process")
				continue
			}

			if err := c.kvController.Delete(log, process.Pid, pAny, 500*time.Millisecond); err != nil {
				log.WithError(err).WithField("pid", process.Pid).Error("reaper: failed to deregister disconnected process")
				continue
			}

			c.broadcaster.PublishRemoved(process.Pid)
			continue
		}

		if sinceLastStatus > staleThreshold {
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
