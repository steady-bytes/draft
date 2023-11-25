package registrar

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	api "github.com/steady-bytes/draft/api/gen/go"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TODO: Move the following methods to a package for reusability

// NewApiToken - Builds a token
func NewProcessFromHandshakePayload(process *api.Process) (*api.Process, error) {
	apiToken := &api.Token{}

	// create uuid for process id
	pid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	apiToken.Id = pid.String()

	// Example: this will give us a 44 byte, base64 encoded output
	apiToken.Nonce, err = GenerateRandomStringURLSafe(32)
	if err != nil {
		// Serve an appropriately vague error to the
		// user, but log the details internally.
		return nil, err
	}

	// Create  new token object, specifying signing method and the claims
	// you would like it to contain.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"registry": pid,
		"nbf":      time.Date(2022, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	apiToken.Jwt, err = token.SignedString([]byte(apiToken.Nonce))
	if err != nil {
		return nil, err
	}

	// set the generated token
	process.Token = apiToken

	// overwrite the process id, with the new generate id
	process.Id = apiToken.GetId()

	return process, nil
}

func (s *service) buildHandshakeInititatedEventRequest(pid string) (*api.EmitEventRequest, error) {
	// init handshake started event
	evtData := &api.HandshakeInitiated{
		ProcessId:     pid,
		LeaderAddress: "http://[::1]:50002",
		InitiatedTime: timestamppb.Now(),
	}

	// marshal event data
	evtDataJson, err := protojson.Marshal(evtData)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	// init event
	evt := &api.Event{
		Id:            uuid.NewString(),
		AggregateId:   pid,
		TransactionId: uuid.NewString(),
		Data:          string(evtDataJson),
		CreatedAt:     timestamppb.Now(),
		AggregateKind: api.AggregateKind_REGISTRY,
		EventCode:     api.EventCode_HANDSHAKE_INITIATED,
		SideAffect:    false,
	}

	// wrap event in req
	return &api.EmitEventRequest{
		Payload: evt,
	}, nil
}

// GenerateRandomBytes returns securely generated random bytes.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		return nil, err
	}

	return b, nil
}

// GenerateRandomStringURLSafe returns a URL-safe, base64 encoded
// securely generated random string.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomStringURLSafe(n int) (string, error) {
	b, err := GenerateRandomBytes(n)
	return base64.URLEncoding.EncodeToString(b), err
}
