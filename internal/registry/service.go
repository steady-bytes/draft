package registry

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	api "github.com/steady-bytes/draft/api/gen/go"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/jinzhu/gorm"
)

type service struct {
	api.RegistryServer
	DB               *gorm.DB
	eventStoreClient api.EventStoreClient
}

func NewService() (*service, error) {
	url := fmt.Sprintf("%s:%d", "localhost", 50001)
	conn, err := grpc.Dial(url, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("[%s] Dial failed: %v", url, err)
		return nil, err
	}

	client := api.NewEventStoreClient(conn)

	return &service{
		eventStoreClient: client,
	}, nil
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

	fmt.Println("payload validated")

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

	// init handshake started event
	evtData := &api.HandshakeInitiated{
		ProcessId:     process.GetId(),
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
		AggregateId:   process.GetId(),
		TransactionId: uuid.NewString(),
		Data:          string(evtDataJson),
		CreatedAt:     timestamppb.Now(),
		AggregateKind: api.AggregateKind_REGISTRY,
		EventCode:     api.EventCode_HANDSHAKE_INITIATED,
		SideAffect:    false,
	}

	// wrap event in req
	esReq := &api.CreateEventRequest{
		Payload: evt,
	}

	fmt.Println("sending event")

	// emit and event to the eventstore service
	res, err := s.eventStoreClient.Create(ctx, esReq)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	fmt.Println("event sent")

	// iniate the `Handshake`
	handshake := &api.Handshake{
		ProcessId: process.GetId(),
		// TODO: change this to some method that will fetch dynamicly
		LeaderAddress: "http://[::1]:50002",
		Token:         process.GetToken(),
		TransactionId: res.GetResult().GetTransactionId(),
	}

	fmt.Println("handshake: ", handshake)

	return handshake, nil
}

// ConnectProcess - Uses the
func (s *service) ConnectProcess(stream api.Registry_ConnectProcessServer) error {
	var processID string
	for {
		msg, closer := stream.Recv()
		if closer == io.EOF {
			return stream.SendAndClose(&api.Empty{})
		}
		if closer != nil {
			fmt.Println("stream closing")

			req, err := s.buildProcessDisconnectedEventRequest(processID)
			if err != nil {
				fmt.Println(err)
				return err
			}

			ctx := context.Background()
			// emit event to the `EventStore`
			res, err := s.eventStoreClient.Create(ctx, req)
			if err != nil {
				fmt.Println(err)
				return err
			}

			// update left_time in the database
			if err := s.updateLeftTimeStamp(ctx, res.GetResult().GetCreatedAt().AsTime(), res.GetResult().GetAggregateId()); err != nil {
				fmt.Println(err)
				return err
			}

			// close the connection by returning the closer
			return closer
		} else {
			fmt.Println("")
			fmt.Println("status: \n", msg)
			processID = msg.GetProcessId()
			if processID == "" {
				msg := "process id is invalid"
				fmt.Println(msg)
				return errors.New(msg)
			}

			// If the `ProcessDetails` contain the correct `nonce`, and `token` then update the `last_status_time` field
			if err := s.updateProcessDetails(msg); err != nil {
				fmt.Println("process details failed to update")

				// emit an event "SYSTEM_UPDATE_FAILUER"

				return err
			}

			// emit event `PROCESS_CONNECTED`
		}
	}
}

func (s *service) buildProcessDisconnectedEventRequest(processID string) (*api.CreateEventRequest, error) {
	// emit event `PROCESS_DISCONNECTED`
	evtData := &api.ProcessDisconnected{
		ProcessId:      processID,
		DisconnectedAt: timestamppb.Now(),
	}

	// marshal event data
	evtDataJson, err := protojson.Marshal(evtData)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	// build event struct
	evt := &api.Event{
		Id:          uuid.NewString(),
		AggregateId: processID,
		// NOTE: this may need to chanage
		TransactionId: uuid.NewString(),
		Data:          string(evtDataJson),
		CreatedAt:     timestamppb.Now(),
		AggregateKind: api.AggregateKind_REGISTRY,
		EventCode:     api.EventCode_PROCESS_DISCONNECTED,
		SideAffect:    false,
	}

	// return built event request
	return &api.CreateEventRequest{
		Payload: evt,
	}, nil
}

func (s *service) updateLeftTimeStamp(ctx context.Context, t time.Time, processID string) error {
	model := &api.ProcessORM{
		Id:           processID,
		LeftTime:     &t,
		RunningState: api.ProcessRunningState_value["PROCESS_DICONNECTED"],
	}

	// find by process id, and update it's values
	db := s.DB.Save(&model)
	if db.Error != nil {
		fmt.Println("when updating the disconnecting process in the db and error occured", db.Error)
		return db.Error
	}

	return nil
}

func (s *service) updateProcessDetails(details *api.ProcessDetails) error {
	p := &api.Process{
		Id: details.GetProcessId(),
	}

	ctx := context.Background()
	var err error
	p, err = api.DefaultReadProcess(ctx, p, s.DB)
	if err != nil {
		fmt.Println("error: ", err)
		return err
	}

	fmt.Println("found process: \n", p)

	model, err := p.ToORM(ctx)
	if err != nil {
		fmt.Println("error converting to orm type", err)
	}

	// if both the `token` and `nonce` are valid, update `last_status_time`
	if model.Token.Jwt == details.GetToken() && model.Token.Nonce == details.GetNonce() {
		model.RunningState = api.ProcessRunningState_value[details.GetRunningState().String()]
		model.ProcessHealth = api.ProcessHealthState_value[details.GetProcessHealth().String()]
		now := time.Now()
		model.LastStatusTime = &now

		db := s.DB.Save(&model)
		if db.Error != nil {
			fmt.Println("when updating the last_status_time and error occured", db.Error)
			return db.Error
		}
	}

	fmt.Println("update process: \n", p)

	return nil
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
		"nbf":      time.Date(2022, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(nonce))
	if err != nil {
		return nil, err
	}

	// set the token as a string
	apiToken.Jwt = tokenString

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
