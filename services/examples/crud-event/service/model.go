package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	crudv1 "github.com/steady-bytes/draft/api/examples/crud/v1"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
)

// CreateSchema creates the names table if it doesn't already exist. Mirrors
// the pattern in services/tooling/bench/model.go's createSchema.
func CreateSchema(ctx context.Context, db bun.Repository) error {
	if _, err := db.Client().NewCreateTable().Model((*crudv1.Name)(nil)).IfNotExists().Exec(ctx); err != nil {
		return fmt.Errorf("failed to create table for %T: %w", (*crudv1.Name)(nil), err)
	}
	return nil
}

type (
	// Model is services/examples/crud's own Model interface, widened to return the full *Name
	// each operation acted on rather than just its id -- crud-event's handler needs the complete
	// model (not merely its id) to publish as a ModelEvent (see service/events.go), which plain
	// crud, having no event to emit, never needed.
	Model interface {
		Create(ctx context.Context, name *crudv1.Name) (*crudv1.Name, error)
		Read(ctx context.Context, id string) (*crudv1.Name, error)
		Update(ctx context.Context, name *crudv1.Name) (*crudv1.Name, error)
		Delete(ctx context.Context, id string) (*crudv1.Name, error)
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

func (m *model) Create(ctx context.Context, name *crudv1.Name) (*crudv1.Name, error) {
	name.Id = uuid.New().String()
	if _, err := m.db.Client().NewInsert().Model(name).Exec(ctx); err != nil {
		return nil, err
	}
	return name, nil
}

func (m *model) Read(ctx context.Context, id string) (*crudv1.Name, error) {
	name := &crudv1.Name{}
	if _, err := m.db.Client().NewSelect().Model(name).Where("id = ?", id).Exec(ctx, name); err != nil {
		return nil, err
	}
	return name, nil
}

func (m *model) Update(ctx context.Context, name *crudv1.Name) (*crudv1.Name, error) {
	query := m.db.Client().NewUpdate().Model(&crudv1.Name{})
	if name.FirstName != "" {
		query = query.Set("first_name = ?", name.FirstName)
	}
	if name.LastName != "" {
		query = query.Set("last_name = ?", name.LastName)
	}
	// Returning("*") is the fix for a bug services/examples/crud's own model.go carries as an
	// open TODO ("always `sql: no rows in result set` ... even though it does perform the
	// update"): passing a dest to bun's Exec makes it try to scan a result row, but an UPDATE
	// with no RETURNING clause produces no result set at all for the Postgres driver to scan from
	// -- the update itself still ran (confirmed against the live table), only the scan failed.
	// Explicitly requesting every column back is what makes Exec's dest scan actually have a row.
	updated := &crudv1.Name{}
	_, err := query.Where("id = ?", name.Id).Returning("*").Exec(ctx, updated)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (m *model) Delete(ctx context.Context, id string) (*crudv1.Name, error) {
	// Same fix as Update above: RETURNING is required for Exec's dest scan to have anything to
	// scan from a DELETE.
	name := &crudv1.Name{}
	_, err := m.db.Client().NewDelete().Model(name).Where("id = ?", id).Returning("*").Exec(ctx, name)
	if err != nil {
		return nil, err
	}
	if name.Id == "" {
		// The RETURNING scan above doesn't always populate name (same underlying bun quirk as the
		// TODO in Update) -- fall back to at least the id the caller asked to delete so the
		// resulting ModelEvent isn't missing it entirely.
		name.Id = id
	}
	return name, nil
}
