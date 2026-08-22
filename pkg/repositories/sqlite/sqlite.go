package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"

	// Pure-Go driver, registers itself under the "sqlite" driver name — no
	// CGO, so cross-compilation for desktop/mobile client targets stays
	// simple. This was Architecture's resolved call over both mattn's CGO
	// driver and the existing badger plugin: Block's access pattern (ordered
	// by position, filtered by document_id, parent-child) is natively
	// relational, and a .sqlite file stays independently user-inspectable
	// and exportable in a way a KV store's blob isn't.
	_ "modernc.org/sqlite"
)

type (
	Repository interface {
		chassis.Repository
		Client() *bun.DB
	}
	repository struct {
		client    *bun.DB
		configKey string
	}
)

// New instantiates a new repository. A call to Open is required before use.
// The configKey parameter dictates which key in the configuration will be read during
// initialization. Default: "repositories.sqlite"
func New(configKey string) Repository {
	if configKey == "" {
		configKey = "repositories.sqlite"
	}
	return &repository{
		configKey: configKey,
	}
}

func (r *repository) Client() *bun.DB {
	return r.client
}

func (r *repository) Open(ctx context.Context, config chassis.Config) error {
	path := config.GetString(fmt.Sprintf("%s.path", r.configKey))
	if path == "" {
		path = config.NodeID() + ".sqlite"
	}
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("failed to open sqlite db at %s: %w", path, err)
	}
	// SQLite allows only one writer at a time; a single pooled connection
	// avoids "database is locked" errors surfacing as flaky query failures
	// under concurrent access, rather than masking them with retry logic.
	sqldb.SetMaxOpenConns(1)
	r.client = bun.NewDB(sqldb, sqlitedialect.New())
	return r.Ping(ctx)
}

func (r *repository) Close(ctx context.Context) error {
	err := r.client.Close()
	if err != nil {
		return fmt.Errorf("failed to close the db connection for disconnect")
	}
	return nil
}

func (r *repository) Ping(ctx context.Context) error {
	err := r.client.PingContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping the db")
	}
	return nil
}
