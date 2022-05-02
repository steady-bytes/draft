package registrar

import (
	"context"
	"errors"
	"fmt"
	"io"

	api "github.com/steady-bytes/draft/api/gen/go"

	"google.golang.org/grpc"

	"github.com/jinzhu/gorm"
)

type registrarController struct {
	api.RegistryServer
	DB               *gorm.DB
	eventStoreClient api.EventStoreClient
}

func NewRegistrarController() (*registrarController, error) {
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
func (s *service) InitiateHandshake(ctx context.Context, handshake *api.RequestHandshake) (*api.Handshake, error) {
	// unpack request payload
	payload := handshake.GetPayload()

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

	req, err := s.buildHandshakeInititatedEventRequest(process.GetId())
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	// emit and event to the eventstore service
	res, err := s.eventStoreClient.Create(ctx, req)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	return &api.Handshake{
		ProcessId: process.GetId(),
		// TODO: change this to some method that will fetch dynamicly
		LeaderAddress: "http://[::1]:50002",
		Token:         process.GetToken(),
		TransactionId: res.GetResult().GetTransactionId(),
	}, nil
}

// ConnectProcess - Acts as an individual process that runs for each connected client process to the registry
func (s *service) ConnectProcess(stream api.Registry_ConnectProcessServer) error {
	pid := ""
	isConnected := false
	for {
		msg, closer := stream.Recv()

		if closer == io.EOF {
			return stream.SendAndClose(&api.Empty{})
		}

		// closer has been activated, close down the the connection and update the data store
		if closer != nil {
			fmt.Println("stream closing")

			if err := s.closeProcess(pid); err != nil {
				fmt.Println("error: ", err)
				return err
			}

			// close the connection by returning the closer
			return closer
		}

		// a status message has been received
		if msg != nil {
			fmt.Println("\n status: ", msg)
			pid = msg.GetProcessId()
			if pid == "" {
				msg := "process id is invalid"
				fmt.Println(msg)
				return errors.New(msg)
			}

			// If the `ProcessDetails` contains the correct `nonce`, and `token` then update the `last_status_time` field
			if err := s.updateProcessDetails(msg); err != nil {
				fmt.Println("process details failed to update")
				// TODO: emit an event "SYSTEM_UPDATE_FAILUER"
				return err
			}

			// if `isConnected` is not true, emit event `PROCESS_CONNECTED` to update the `EventStore`
			// this is also considered the first message received on the rpc stream
			if !isConnected {
				req, err := s.buildProcessConnectedEventRequest(pid)
				if err != nil {
					fmt.Println("error: ", err)
					return err
				}

				ctx := context.Background()
				// emit event to the `EventStore`
				// TODO: determin what to do with the create event response, if anything needs to be done
				_, err = s.eventStoreClient.Create(ctx, req)
				if err != nil {
					fmt.Println(err)
					return err
				}
			}
			// set to true, given all required actions for the connection to be established have succeded
			isConnected = true
		}
		// return nothing since we want to stay in the forever loop if nothing bad has happend
	}
}

func (s *service) Disconnect(ctx context.Context, req *api.DisconnectRequest) (*api.Disconnected, error) {
	return nil, errors.New("implement me")
}

func (s *service) Monitor(req *api.MonitorRequest, stream api.Registry_MonitorServer) error {
	return errors.New("implement me")
}

// QuerySystemJournal - Handles three different kinds of quries. `getProcessById`, `getProcessByGroup`, and `getAllProcesses`.
func (s *service) QuerySystemJournal(ctx context.Context, req *api.JournalQueryRequest) (*api.JournalQueryResponse, error) {
	switch req.GetQuery().GetOption().(type) {
	case *api.Query_Id:
		return s.getProcessById(ctx, req.GetQuery().GetId())
	case *api.Query_Group:
		return s.getProcessesByGroup(ctx, req.GetQuery().GetGroup())
	case *api.Query_All:
		return s.getAllProcesses(ctx)
	default:
		return nil, errors.New("query type not handled")
	}
}
