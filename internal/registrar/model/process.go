package model

import (
	"fmt"
	"time"

	api "github.com/steady-bytes/draft/api/go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Process struct {
	// bun.BaseModel `bun:"table:Processes"`

	Id             string      `bun:"id,pk,type:uuid"`
	Name           string      `bun:"name,notnull"`
	Group          string      `bun:",notnull"`
	Local          string      `bun:",notnull"`
	IpAddress      string      `bun:",notnull"`
	ProcessKind    string      `bun:",notnull"`
	Tags           []*Metadata `bun:"rel:has-many,join:id=pid"`
	JoinedTime     time.Time
	LeftTime       time.Time
	LastStatusTime time.Time
	Version        string `bun:",notnull"`
	RunningState   string `bun:",notnull"`
	HealthState    string `bun:",notnull"`
	Token          *Token `bun:"rel:has-one, join:id=pid"`
}

func (p *Process) ToPB() *api.Process {
	return &api.Process{
		Id:             p.Id,
		Name:           p.Name,
		Group:          p.Group,
		Local:          p.Local,
		IpAddress:      p.IpAddress,
		ProcessKind:    api.ProcessKind(api.ProcessKind_value[p.ProcessKind]),
		Tags:           p.TagsFromManyRelation(),
		JoinedTime:     timestamppb.New(p.JoinedTime),
		LeftTime:       timestamppb.New(p.LeftTime),
		LastStatusTime: timestamppb.New(p.LastStatusTime),
		Version:        p.Version,
		RunningState:   api.ProcessRunningState(api.ProcessRunningState_value[p.RunningState]),
		HealthState:    api.ProcessHealthState(api.ProcessHealthState_value[p.HealthState]),
		Token:          p.Token.ToPB(),
	}
}

func (p *Process) TagsFromManyRelation() []*api.Metadata {
	metadata := []*api.Metadata{}

	for _, v := range p.Tags {
		metadata = append(metadata, v.ToPB())
	}

	return metadata
}

func ProcessFromPB(pb *api.Process) (*Process, error) {
	if err := pb.Validate(); err != nil {
		fmt.Println("process pb is invalid", err)
		return nil, err
	}

	tags, err := MetadataFromPBManyRelation(pb.GetId(), pb.GetTags())
	if err != nil {
		fmt.Println("failed to convert metadata pb to Metadata")
		return nil, err
	}

	token, err := TokenFromPB(pb.GetId(), pb.GetToken())
	if err != nil {
		fmt.Println("failed to convert token pb to Token")
		return nil, err
	}

	p := &Process{
		Id:             pb.GetId(),
		Name:           pb.GetName(),
		Group:          pb.GetGroup(),
		Local:          pb.GetLocal(),
		IpAddress:      pb.GetIpAddress(),
		ProcessKind:    api.ProcessKind_name[int32(pb.GetProcessKind())],
		Tags:           tags,
		JoinedTime:     pb.GetJoinedTime().AsTime(),
		LeftTime:       pb.GetLeftTime().AsTime(),
		LastStatusTime: pb.GetLastStatusTime().AsTime(),
		Version:        pb.GetVersion(),
		RunningState:   api.ProcessRunningState_name[int32(pb.GetRunningState())],
		HealthState:    api.ProcessHealthState_name[int32(pb.GetHealthState())],
		Token:          token,
	}

	return p, nil
}
