package service_discovery

import (
	"context"
	"errors"
	"time"

	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"
	kv "github.com/steady-bytes/draft/blueprint/key_value"
	"github.com/steady-bytes/draft/pkg/chassis"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"
)

const (
	// TODO: this was removed from the chassis and I'm not sure where it should go tbh
	GlobalNonceKey = "GLOBAL_NONCE"
)

type (
	Controller interface {
		ServiceDiscovery
	}

	ServiceDiscovery interface {
		Finalize(ctx context.Context, log chassis.Logger, pid string) error
		Initialize(ctx context.Context, log chassis.Logger, nonce, name string) (*sdv1.ProcessIdentity, error)
		Synchronize(ctx context.Context, log chassis.Logger, details *sdv1.ClientDetails)
	}

	controller struct {
		kvController kv.Controller
		secretStore  chassis.SecretStore
	}
)

func NewController(kvController kv.Controller) Controller {
	return &controller{
		kvController: kvController,
		secretStore:  nil,
	}
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
	ErrFailedTokenForge               = "failed to forge the token"
	ErrFailedTypeCast                 = "failed to cast type"
)

// Initialize - When a service starts and wants to register itself with the system then a unique name, and system nonce
// can be provided to get `ProcessIdentity` details so that A process can then finalize service registration
func (c *controller) Initialize(ctx context.Context, log chassis.Logger, nonce, name string) (*sdv1.ProcessIdentity, error) {
	var (
		err     error
		pid     = uuid.NewString()
		process = &sdv1.Process{
			Pid:          pid,
			Name:         name,
			ProcessKind:  sdv1.ProcessKind_SERVER_PROCESS,
			Metadata:     []*sdv1.Metadata{},
			JoinedTime:   timestamppb.Now(),
			RunningState: sdv1.ProcessRunningState_PROCESS_STARTING,
			HealthState:  sdv1.ProcessHealthState_PROCESS_HEALTHY,
		}
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

	// generate the process identity as a signed token token
	process.Token, err = c.forgeIdentityToken()
	if err != nil {
		return nil, errors.New(ErrFailedTokenForge)
	}

	pAny, err = anypb.New(process)
	if err != nil {
		return nil, errors.New(ErrFailedTypeCast)
	}

	// save details to the systemJournal on the file system
	_, err = c.kvController.Set(log, pid, pAny, 500*time.Millisecond)
	if err != nil {
		return nil, errors.New(ErrFailedToSaveProcessDetails)
	}

	// TODO (@andrewsc208): Get the leaders registry address to send synchronize packets to

	return &sdv1.ProcessIdentity{
		Pid:             pid,
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
}

// Finalize - Gracefully remove the process from the registry. Close the connection if one is still
// open and change the process state to `Finalized`
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

	if err = c.kvController.Delete(log, pid, pAny); err != nil {
		log.WithError(err)
		return err
	}

	return nil
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
