package model

import (
	"errors"

	"github.com/uptrace/bun"

	draft "github.com/steady-bytes/draft/pkg/chassis"
)

type (
	TestModel interface {
		draft.RepoRegistrar
		SaveTest() error
	}

	testModel struct {
		db *bun.DB
	}
)

func NewTestModel() TestModel {
	return &testModel{}
}

func (r *testModel) RegisterRepo(dbConn interface{}) error {
	if dbConn == nil {
		return errors.New("db interface is nil")
	} else {
		if db, ok := dbConn.(*bun.DB); ok {
			r.db = db
		}
	}

	return nil
}

func (r *testModel) SaveTest() error {
	return errors.New("implement me")
}
