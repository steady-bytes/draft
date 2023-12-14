package model

import (
	"fmt"

	api "github.com/steady-bytes/draft/api/go"
)

type Token struct {
	Id    string `bun:"id,pk,type:uuid"`
	Pid   string `bun:",type:uuid"`
	Jwt   string `bun:",notnull"`
	Nonce string `bun:",notnull"`
}

func (t *Token) ToPB() *api.Token {
	return &api.Token{
		Id:    t.Id,
		Jwt:   t.Jwt,
		Nonce: t.Nonce,
	}
}

func TokenFromPB(fk string, pb *api.Token) (*Token, error) {
	if err := pb.Validate(); err != nil {
		fmt.Println("token pb is invalid", err)
		return nil, err
	}

	return &Token{
		Id:    pb.GetId(),
		Pid:   fk,
		Jwt:   pb.GetJwt(),
		Nonce: pb.GetNonce(),
	}, nil
}
