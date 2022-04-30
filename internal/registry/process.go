package registry

import (
	"context"
	"errors"
	"fmt"
	"time"

	api "github.com/steady-bytes/draft/api/gen/go"

	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *service) closeProcess(pid string) error {
	req, err := s.buildProcessDisconnectedEventRequest(pid)
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

	return nil
}

func (s *service) buildProcessConnectedEventRequest(processID string) (*api.CreateEventRequest, error) {
	// emit event `PROCESS_CONNECTED`
	evtData := &api.ProcessConnected{
		ProcessId:   processID,
		ConnectedAt: timestamppb.Now(),
	}

	// marshal event data
	evtDataJson, err := protojson.Marshal(evtData)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	// build event struct
	evt := &api.Event{
		Id:            uuid.NewString(),
		AggregateId:   processID,
		TransactionId: uuid.NewString(),
		Data:          string(evtDataJson),
		CreatedAt:     timestamppb.Now(),
		AggregateKind: api.AggregateKind_REGISTRY,
		EventCode:     api.EventCode_PROCESS_CONNECTED,
		SideAffect:    false,
	}

	// return built event request
	return &api.CreateEventRequest{
		Payload: evt,
	}, nil
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
		Id:            uuid.NewString(),
		AggregateId:   processID,
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
		RunningState: int32(api.ProcessRunningState_PROCESS_DICONNECTED),
		HealthState:  int32(api.ProcessHealthState_PROCESS_UNHEALTHY),
	}

	// find by process id, and update it's values
	db := s.DB.Model(&model).Updates(*model)
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
		model.RunningState = int32(details.GetRunningState())
		model.HealthState = int32(details.GetHealthState())
		now := time.Now()
		model.LastStatusTime = &now

		db := s.DB.Model(&model).Updates(model)
		if db.Error != nil {
			fmt.Println("when updating the last_status_time and error occured", db.Error)
			return db.Error
		}
	}

	fmt.Println("update process: \n", p)

	return nil
}

func (s *service) getProcessById(ctx context.Context, pid string) (*api.JournalQueryResponse, error) {
	p := &api.Process{
		Id: pid,
	}

	// TODO: change this to use gorm directly
	p, err := api.DefaultReadProcess(ctx, p, s.DB)
	if err != nil {
		fmt.Println("error: ", err)
		return nil, err
	}

	result := make(map[string]*api.Process)
	result[pid] = p

	return &api.JournalQueryResponse{
		Result: result,
	}, nil
}

func (s *service) getProcessesByGroup(ctx context.Context, group string) (*api.JournalQueryResponse, error) {
	return nil, errors.New("implement query by group")
}

func (s *service) getAllProcesses(ctx context.Context) (*api.JournalQueryResponse, error) {
	return nil, errors.New("implement query all processes")
}
