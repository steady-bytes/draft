package registry

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	api "github.com/steady-bytes/draft/api/gen/go"

	"github.com/jinzhu/gorm"
)

type service struct {
	api.RegistryServer
	DB *gorm.DB
}

func NewService() *service {
	return &service{}
}

const clientID = "78f5b6e1-3096-4d40-8bdc-8061d2cc0751"

// InitiateHandshake -
func (s *service) InitiateHandshake(ctx context.Context, req *api.RequestHandshake) (*api.Handshake, error) {
	// unpack request payload
	payload := req.GetPayload()

	// validate
	if err := payload.Validate(); err != nil {
		msg := fmt.Sprintf("payload is not valid %s", err)
		return nil, errors.New(msg)
	}

	// check the ProcessId to make sure that it's equal to the client_id
	// TODO: change this to some method to get dynamictly
	if payload.GetId() != clientID {
		return nil, errors.New("internal error, invalid client id")
	}

	// create api token
	process, err := NewProcessFromHandshakePayload(payload)
	if err != nil {
		return nil, err
	}

	// store in db
	process, err = api.DefaultCreateProcess(ctx, process, s.DB)
	if err != nil {
		msg := fmt.Sprintf("payload was not stored %s", err)
		return nil, errors.New(msg)
	}

	// send event?
	// process joined

	// iniate the `Handshake` type
	handshake := &api.Handshake{
		ProcessId: process.GetId(),
		// TODO: change this to some method that will fetch dynamicly
		LeaderAddress: "http://[::1]:50002",
		Token:         process.GetToken(),
	}

	return handshake, nil
}

func (s *service) Connect(stream api.Registry_ConnectServer) error {
	return errors.New("implement me")
}

func (s *service) Disconnect(ctx context.Context, req *api.DisconnectRequest) (*api.Disconnected, error) {
	return nil, errors.New("implement me")
}

func (s *service) Monitor(req *api.MonitorRequest, stream api.Registry_MonitorServer) error {
	return errors.New("implement me")
}

func (s *service) QuerySystemJournal(ctx context.Context, req *api.JournalQueryRequest) (*api.JournalQueryResponse, error) {
	return nil, errors.New("implement me")
}

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
	nonce, err := GenerateRandomStringURLSafe(32)
	if err != nil {
		// Serve an appropriately vague error to the
		// user, but log the details internally.
		return nil, err
	}

	apiToken.Nonce = nonce

	// Create  new token object, specifying signing method and the claims
	// you would like it to contain.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"registry": pid,
		"nbf":      time.Date(2015, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(nonce))
	if err != nil {
		return nil, err
	}

	// set the token as a string
	apiToken.Token = tokenString

	// set the generated token
	process.Token = apiToken

	// overwrite the process id, with the new generate id
	process.Id = apiToken.GetId()

	return process, nil
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
