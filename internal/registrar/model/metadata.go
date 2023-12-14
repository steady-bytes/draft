package model

import (
	"fmt"

	api "github.com/steady-bytes/draft/api/go"
)

type Metadata struct {
	Id    string `bun:"id,pk,type:uuid"`
	Pid   string `bun:",type:uuid"`
	Key   string `bun:",notnull"`
	Value string `bun:",notnull"`
}

func (m *Metadata) ToPB() *api.Metadata {
	return &api.Metadata{
		Id:    m.Id,
		Key:   m.Key,
		Value: m.Value,
	}
}

func MetadataFromPB(fk string, pb *api.Metadata) (*Metadata, error) {
	if err := pb.Validate(); err != nil {
		fmt.Println("metadata pb is invalid", err)
		return nil, err
	}

	return &Metadata{
		Id:    pb.GetId(),
		Pid:   fk,
		Key:   pb.GetKey(),
		Value: pb.GetValue(),
	}, nil
}

func MetadataFromPBManyRelation(fk string, pbs []*api.Metadata) ([]*Metadata, error) {
	tags := []*Metadata{}

	for _, v := range pbs {
		t, err := MetadataFromPB(fk, v)
		if err != nil {
			fmt.Println("error converting pb metadata to Metadata")
			return nil, err
		} else {
			tags = append(tags, t)
		}
	}

	return tags, nil
}
