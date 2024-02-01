package service_discovery

import (
	"context"
	"errors"
	"fmt"
	"time"

	sdv1 "github.com/steady-bytes/draft/api/registry/service_discovery/v1"
	kv "github.com/steady-bytes/draft/blueprint/key_value"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
	"github.com/steady-bytes/draft/pkg/logging"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"
)

type (
	Controller interface {
		draft.SecretStoreSetter

		ServiceDiscovery
	}

	ServiceDiscovery interface {
		Finalize(ctx context.Context, pid string) error
		Initialize(ctx context.Context, log logging.Logger, nonce, name string) (*sdv1.ProcessIdentity, error)
		Synchronize(ctx context.Context, log logging.Logger, details *sdv1.ClientDetails)
	}

	controller struct {
		kvController kv.Controller
		secretStore  draft.SecretStore
	}
)

func NewController(kvController kv.Controller) Controller {
	return &controller{
		kvController: kvController,
		secretStore:  nil,
	}
}

// Accepts a `SecretStore` interface and adds it to the controller
func (c *controller) SetSecretStore(s draft.SecretStore) {
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
func (c *controller) Initialize(ctx context.Context, log logging.Logger, nonce, name string) (*sdv1.ProcessIdentity, error) {
	var (
		pid = uuid.NewString()
	)

	// validate the nonce (this will also require that a nonce is read in by the golang-draft-runtime).
	n, err := c.secretStore.Get(draft.GlobalNonceKey)
	if err != nil || n != nonce {
		return nil, errors.New(ErrFailedNonce)
	}

	p, err := anypb.New(&sdv1.Process{})
	if err != nil {
		return nil, errors.New(ErrFailedTypeCast)
	}

	// check to see if the name already exists
	_, err = c.kvController.Get(name, p)
	if err == nil {
		return nil, errors.New(ErrFailedProcessAlreadyRegistered)
	}

	// generate the process identity as a signed token token
	token, err := c.forgeIdentityToken()
	if err != nil {
		return nil, errors.New(ErrFailedTokenForge)
	}

	pc, err := anypb.New(&sdv1.Process{
		Pid:          pid,
		Name:         name,
		ProcessKind:  sdv1.ProcessKind_SERVER_PROCESS,
		Metadata:     []*sdv1.Metadata{},
		JoinedTime:   timestamppb.Now(),
		RunningState: sdv1.ProcessRunningState_PROCESS_STARTING,
		HealthState:  sdv1.ProcessHealthState_PROCESS_HEALTHY,
		Token:        token,
	})
	if err != nil {
		return nil, errors.New(ErrFailedTypeCast)
	}

	// save details to the systemJournal on the file system
	_, err = c.kvController.Set(log, pid, pc, 500*time.Millisecond)
	if err != nil {
		return nil, errors.New(ErrFailedToSaveProcessDetails)
	}

	// TODO -> Get the leaders address to send synchronize packets to

	return &sdv1.ProcessIdentity{
		Pid:             pid,
		RegistryAddress: "localhost:2221",
		Token:           token,
	}, nil
}

// Synchronize - receive a message from an `Initialized` process and update it's state in the
// `SystemJournal`.
func (c *controller) Synchronize(ctx context.Context, log logging.Logger, details *sdv1.ClientDetails) {
	// Look for the key, if not found return error
	process := &sdv1.Process{}
	pAny, err := anypb.New(process)
	if err != nil {
		log.WithError(kv.ErrFailedAnyCast)
		return
	}

	p, err := c.kvController.Get(details.Pid, pAny)
	if err != nil {
		log.WithError(err)
		return
	}

	fmt.Println(p.GetValue())

	m := new(sdv1.Process)
	if p.MessageIs(m) {
		fmt.Println("correct type")

		if err := anypb.UnmarshalTo(p, m, proto.UnmarshalOptions{}); err != nil {
			fmt.Println("error: ", err)
		}
	}

	fmt.Println("process: ", m)

	// how to I unmarshal the type that was found in the key/val store

	// ignore if the wrong token is sent
	if m.Token.GetJwt() != details.Token {
		return
	}

	m.HealthState = details.HealthState
	m.Location = details.Location
	m.Metadata = details.Metadata
	m.ProcessKind = details.ProcessKind
	m.RunningState = details.RunningState
	m.LastStatusTime = timestamppb.Now()

	// TODO -> resume here

	// _, err = c.kvController.Set(process.Pid, process, 500*time.Millisecond)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
}

// Finalize - Gracefully remove the process from the registry. Close the connection if one is still
// open and change the process state to `Finalized`
func (c *controller) Finalize(ctx context.Context, pid string) error {
	// if err := c.kvController.Delete(pid, &sdv1.Process{}); err != nil {
	// 	return err
	// }

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
