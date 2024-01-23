package service_discovery

import (
	"context"
	"errors"

	sdv1 "github.com/steady-bytes/draft/api/registry/service_discovery/v1"
	kv "github.com/steady-bytes/draft/blueprint/key_value"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	"github.com/google/uuid"
)

type (
	Controller interface {
		draft.SecretStoreSetter

		ServiceDiscovery
	}

	ServiceDiscovery interface {
		Finalize(ctx context.Context, pid string) error
		Initialize(ctx context.Context, nonce, name string) (*sdv1.ProcessIdentity, error)
		Synchronize(ctx context.Context, details *sdv1.ClientDetails)
		// Query(ctx context.Context)

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
	ErrfailedNonce                    = "nonce failure"
	ErrfailedProcessAlreadyRegistered = "process has already be initialized"
	ErrfailedToMarshalPayload         = "failed to marshal payload"
	ErrfailedToSaveProcessDetails     = "failed to save process details"
	ErrfailedTokenForge               = "failed to forge the token"
)

// Finalize - Gracefully remove the process from the registry. Close the connection if one is still
// open and change the process state to `Finalized`
func (c *controller) Finalize(ctx context.Context, pid string) error {
	// if err := c.kvController.Delete(pid, &sdv1.Process{}); err != nil {
	// 	return err
	// }

	return nil
}

// Initialize - When a service starts and wants to register itself with the system then a unique name, and system nonce
// can be provided to get `ProcessIdentity` details so that A process can then finalize service registration
func (c *controller) Initialize(ctx context.Context, nonce, name string) (*sdv1.ProcessIdentity, error) {
	// var (
	// 	pid = uuid.NewString()
	// )

	// // validate the nonce (this will also require that a nonce is read in by the golang-draft-runtime).
	// n, err := c.sstr.Get(draft.GlobalNonceKey)
	// if err != nil || n != nonce {
	// 	return nil, errors.New(ErrfailedNonce)
	// }

	// // check to see if the name already exists
	// // TODO -> I think this should be like check key, even though it would be just another
	// // wrapper around `Get`. In this case it seems more semantically correct.
	// _, err = c.Get(name)
	// if err == nil {
	// 	return nil, errors.New(ErrfailedProcessAlreadyRegistered)
	// }

	// token, err := c.generateToken()
	// if err != nil {
	// 	return nil, errors.New(ErrfailedTokenForge)
	// }

	// value := &sdv1.Process{
	// 	Pid:          pid,
	// 	Name:         name,
	// 	ProcessKind:  sdv1.ProcessKind_SERVER_PROCESS,
	// 	Metadata:     []*sdv1.Metadata{},
	// 	JoinedTime:   timestamppb.Now(),
	// 	RunningState: sdv1.ProcessRunningState_PROCESS_STARTING,
	// 	HealthState:  sdv1.ProcessHealthState_PROCESS_HEALTHY,
	// 	Token:        token,
	// }

	// // save details to the systemJournal on the file system
	// _, err = c.Set(pid, value, 500*time.Millisecond)
	// if err != nil {
	// 	return nil, errors.New(ErrfailedToSaveProcessDetails)
	// }

	// return &sdv1.ProcessIdentity{
	// 	Pid:             pid,
	// 	RegistryAddress: "localhost:2221",
	// 	Token:           token,
	// }, nil

	return nil, errors.New("implement me")
}

// Synchronize - receive a message from an `Initialized` process and update it's state in the
// `SystemJournal`.
func (c *controller) Synchronize(ctx context.Context, details *sdv1.ClientDetails) {
	// Look for the key, if not found return error
	// _, err := c.kvController.Get(details.Pid)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }

	// process := &sdv1.Process{}
	// if err := json.Unmarshal([]byte(byt.Value), process); err != nil {
	// 	fmt.Println(err)
	// 	return
	// }

	// // ignore if the wrong token is sent
	// if process.Token.Jwt != details.Token {
	// 	return
	// }

	// process.HealthState = details.HealthState
	// process.Location = details.Location
	// process.Metadata = details.Metadata
	// process.ProcessKind = details.ProcessKind
	// process.RunningState = details.RunningState
	// process.LastStatusTime = timestamppb.Now()

	// _, err = c.Set(process.Pid, process, 500*time.Millisecond)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
}

// TODO -> Figure out how I want to generate a token for the process
// Right now just return test
func (c *controller) generateToken() (*sdv1.Token, error) {
	// t := jwt.New(jwt.GetSigningMethod("RS256"))
	// return t.SignedString(signKey)

	return &sdv1.Token{
		Id:  uuid.NewString(),
		Jwt: "test",
	}, nil
}

func (c *controller) Query(ctx context.Context) {
	// values, err := c.kvController.Query(&anypb.Any{})
	// if err != nil {
	// 	fmt.Println(err)
	// }

	// for k, v := range values {
	// 	fmt.Println(k, v)
	// }
}
