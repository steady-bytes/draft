package service

import (
	"context"

	"github.com/google/uuid"
	crudv1 "github.com/steady-bytes/draft/api/examples/crud/v1"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
)

type (
	Model interface {
		Create(ctx context.Context, name *crudv1.Name) (id string, err error)
		Read(ctx context.Context, id string) (name *crudv1.Name, err error)
		Update(ctx context.Context, name *crudv1.Name) (id string, err error)
		Delete(ctx context.Context, id string) (err error)
	}
	model struct {
		db bun.Repository
	}
)

func NewModel(db bun.Repository) Model {
	return &model{
		db: db,
	}
}

func (m *model) Create(ctx context.Context, name *crudv1.Name) (id string, err error) {
	name.Id = uuid.New().String()
	_, err = m.db.Client().NewInsert().Model(name).Exec(ctx)
	if err != nil {
		return "", err
	}
	return name.Id, nil
}

func (m *model) Read(ctx context.Context, id string) (name *crudv1.Name, err error) {
	name = &crudv1.Name{}
	_, err = m.db.Client().NewSelect().Model(name).Where("id = ?", id).Exec(ctx, name)
	if err != nil {
		return nil, err
	}
	return name, nil
}

func (m *model) Update(ctx context.Context, name *crudv1.Name) (id string, err error) {
	query := m.db.Client().NewUpdate().Model(&crudv1.Name{})
	if name.FirstName != "" {
		query = query.Set("first_name = ?", name.FirstName)
	}
	if name.LastName != "" {
		query = query.Set("last_name = ?", name.LastName)
	}
	_, err = query.Where("id = ?", name.Id).Exec(ctx, &crudv1.Name{})
	if err != nil {
		// TODO: this is always `sql: no rows in result set` for some reason even though it does perform the update
		return "", err
	}
	return name.Id, nil
}

func (m *model) Delete(ctx context.Context, id string) (err error) {
	name := &crudv1.Name{}
	_, err = m.db.Client().NewDelete().Model(name).Where("id = ?", id).Exec(ctx, name)
	if err != nil {
		// TODO: this is always `sql: no rows in result set` for some reason even though it does perform the delete
		return err
	}
	return nil
}
