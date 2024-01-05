package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	sdv1 "github.com/steady-bytes/draft/api/gen/go/registry/service_discovery/v1"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	ServiceDiscovery interface {
		Finalize(ctx context.Context, pid string) error
		Initialize(ctx context.Context, nonce, name string) (*sdv1.ProcessIdentity, error)
		Synchronize(ctx context.Context, details *sdv1.ClientDetails)
		Query(ctx context.Context)
	}
)

const (
	signKey                        = "TODO -> load this from the secret store"
	failedNonce                    = "nonce failure"
	failedProcessAlreadyRegistered = "process has already be initialized"
	failedToMarshalPayload         = "failed to marshal payload"
	failedToSaveProcessDetails     = "failed to save process details"
	failedTokenForge               = "failed to forge the token"
)

// Finalize - Remove the process from the registry
func (c *controller) Finalize(ctx context.Context, pid string) error {
	if err := c.Delete(pid); err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

// Init - When a service starts and wants to register itself with the system then a uniqu name, and system nonce
// can be provided to get `ProcessIdentity` details so that A process can then finalize service registration
func (c *controller) Initialize(ctx context.Context, nonce, name string) (*sdv1.ProcessIdentity, error) {
	var (
		pid = uuid.NewString()
	)

	// validate the nonce (this will also require that a nonce is read in by the golang-draft-runtime).
	n, err := c.secretStore.Get(draft.GlobalNonceKey)
	if err != nil || n != nonce {
		fmt.Println(failedNonce)
		return nil, errors.New(failedNonce)
	}

	// check to see if the name already exists
	_, err = c.Get(c.keyName(name))
	if err == nil {
		// if this was not found then proceed
		fmt.Println(failedProcessAlreadyRegistered)
		return nil, errors.New(failedProcessAlreadyRegistered)
	}

	jwt, err := c.generateJWTToken()
	if err != nil {
		fmt.Println(err)
		return nil, errors.New(failedTokenForge)
	}

	token := &sdv1.Token{
		Id:  uuid.NewString(),
		Jwt: jwt,
	}

	value := &sdv1.Process{
		Pid:          pid,
		Name:         name,
		ProcessKind:  sdv1.ProcessKind_SERVER_PROCESS,
		Metadata:     []*sdv1.Metadata{},
		JoinedTime:   timestamppb.Now(),
		RunningState: sdv1.ProcessRunningState_PROCESS_STARTING,
		HealthState:  sdv1.ProcessHealthState_PROCESS_HEALTHY,
		Token:        token,
	}

	payload := &CommandPayload{
		Operation: Set,
		Key:       c.keyName(pid),
		Value:     value,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return nil, errors.New(failedToMarshalPayload)
	}

	// save details to the systemJournal on the file system
	_, err = c.Set(data, 500*time.Millisecond)
	if err != nil {
		fmt.Println(err)
		return nil, errors.New(failedToSaveProcessDetails)
	}

	return &sdv1.ProcessIdentity{
		Pid:             pid,
		RegistryAddress: "localhost:2221",
		Token:           token,
	}, nil
}

func (c *controller) keyName(key string) string {
	return "sd-" + key
}

func (c *controller) Synchronize(ctx context.Context, details *sdv1.ClientDetails) {
	// Look for the key, if not found return error
	byt, err := c.Get(c.keyName(details.Pid))
	if err != nil {
		fmt.Println(err)
		return
	}

	process := &sdv1.Process{}
	if err := json.Unmarshal(byt, process); err != nil {
		fmt.Println(err)
		return
	}

	// ignore if the wrong token is sent
	if process.Token.Jwt != details.Token {
		return
	}

	process.HealthState = details.HealthState
	process.Location = details.Location
	process.Metadata = details.Metadata
	process.ProcessKind = details.ProcessKind
	process.RunningState = details.RunningState
	process.LastStatusTime = timestamppb.Now()

	payload := &CommandPayload{
		Operation: Set,
		Key:       c.keyName(process.Pid),
		Value:     process,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return
	}

	_, err = c.Set(data, 500*time.Millisecond)
	if err != nil {
		fmt.Println(err)
		return
	}
}

// TODO -> Figure out how I want to generate a token for the process
// Right now just return test
func (c *controller) generateJWTToken() (string, error) {
	// t := jwt.New(jwt.GetSigningMethod("RS256"))
	// return t.SignedString(signKey)
	return "test", nil
}

func (c *controller) Query(ctx context.Context) {
	c.db.Iterate([]byte("sd-"))
}
