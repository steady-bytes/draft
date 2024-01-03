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
		Init(ctx context.Context, nonce, name string) (*sdv1.ProcessIdentity, error)
	}
)

const (
	signKey                        = "TODO -> load this from the secret store"
	failedNonce                    = "nonce failuer"
	failedProcessAlreadyRegistered = "process has already be initialized"
	failedToMarshalPayload         = "failed to marshal payload"
	failedToSaveProcessDetails     = "failed to save process details"
	failedTokenForge               = "failed to forge the token"
)

// Init - When a service starts and wants to register itself with the system then a uniqu name, and system nonce
// can be provided to get `ProcessIdentity` details so that A process can then finalize service registration
func (c *controller) Init(ctx context.Context, nonce, name string) (*sdv1.ProcessIdentity, error) {
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
	_, err = c.Get(name)
	if err == nil {
		// if this was not found then procced
		fmt.Println(failedProcessAlreadyRegistered)
		return nil, errors.New(failedProcessAlreadyRegistered)
	}

	jwt, err := c.generateJWTToken()
	if err != nil {
		fmt.Println(err)
		return nil, errors.New(failedTokenForge)
	}

	token := &sdv1.Token{
		Id:    uuid.NewString(),
		Jwt:   jwt,
		Nonce: nonce,
	}

	value := &sdv1.Process{
		Pid:          pid,
		Name:         name,
		ProcessKind:  sdv1.ProcessKind_SERVER_PROCESS,
		Tags:         []*sdv1.Metadata{},
		JoinedTime:   timestamppb.Now(),
		RunningState: sdv1.ProcessRunningState_PROCESS_STARTING,
		HealthState:  sdv1.ProcessHealthState_PROCESS_HEALTHY,
		Token:        token,
	}

	payload := &CommandPayload{
		Operation: Set,
		Key:       name,
		Value:     value,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return nil, errors.New(failedToMarshalPayload)
	}

	// save details to the filestore
	_, err = c.Set(data, 500*time.Millisecond)
	if err != nil {
		fmt.Println(err)
		return nil, errors.New(failedToSaveProcessDetails)
	}

	// add the process to the `SystemJournal`
	c.systemJournal.Set(name, value)

	return nil, nil
}

func (c *controller) generateJWTToken() (string, error) {
	// t := jwt.New(jwt.GetSigningMethod("RS256"))
	// return t.SignedString(signKey)
	return "", nil
}
