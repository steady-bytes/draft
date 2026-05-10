package broker

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	"google.golang.org/protobuf/proto"
)

type (
	ClickHouseConfig struct {
		Enabled  bool   `mapstructure:"enabled"`
		Address  string `mapstructure:"address"`
		Database string `mapstructure:"database"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}

	Storer interface {
		Save(ctx context.Context, event *acv1.CloudEvent) error
		Query(ctx context.Context, limit int32, after string) ([]*acv1.CloudEvent, error)
	}

	noopStore struct{}

	clickhouseStore struct {
		conn driver.Conn
	}
)

func NewNoopStore() Storer { return &noopStore{} }

func (n *noopStore) Save(_ context.Context, _ *acv1.CloudEvent) error { return nil }

func (n *noopStore) Query(_ context.Context, _ int32, _ string) ([]*acv1.CloudEvent, error) {
	return nil, nil
}

func NewClickHouseStore(cfg ClickHouseConfig) (Storer, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Address},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	store := &clickhouseStore{conn: conn}
	if err := store.migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return store, nil
}

const createEventsTable = `
CREATE TABLE IF NOT EXISTS events (
    id           String,
    source       String,
    spec_version String,
    type         String,
    body         String,
    inserted_at  DateTime64(9) DEFAULT now64(),
    raw          String
) ENGINE = MergeTree()
ORDER BY (type, inserted_at)
PARTITION BY toYYYYMM(inserted_at)
`

func (s *clickhouseStore) migrate(ctx context.Context) error {
	if err := s.conn.Exec(ctx, createEventsTable); err != nil {
		return err
	}
	// Add body column to existing tables created before this migration.
	return s.conn.Exec(ctx, `ALTER TABLE events ADD COLUMN IF NOT EXISTS body String DEFAULT ''`)
}

func (s *clickhouseStore) Save(ctx context.Context, event *acv1.CloudEvent) error {
	b, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return s.conn.Exec(ctx,
		"INSERT INTO events (id, source, spec_version, type, body, raw) VALUES (?, ?, ?, ?, ?, ?)",
		event.GetId(),
		event.GetSource(),
		event.GetSpecVersion(),
		event.GetType(),
		event.GetTextData(),
		base64.StdEncoding.EncodeToString(b),
	)
}

func (s *clickhouseStore) Query(ctx context.Context, limit int32, after string) ([]*acv1.CloudEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	var (
		rows driver.Rows
		err  error
	)
	if after != "" {
		t, parseErr := time.Parse(time.RFC3339, after)
		if parseErr != nil {
			return nil, fmt.Errorf("parse after timestamp: %w", parseErr)
		}
		rows, err = s.conn.Query(ctx,
			"SELECT raw FROM events WHERE inserted_at > ? ORDER BY inserted_at ASC LIMIT ?",
			t, limit,
		)
	} else {
		rows, err = s.conn.Query(ctx,
			"SELECT raw FROM events ORDER BY inserted_at ASC LIMIT ?",
			limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []*acv1.CloudEvent
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		b, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("decode event: %w", err)
		}
		event := &acv1.CloudEvent{}
		if err := proto.Unmarshal(b, event); err != nil {
			return nil, fmt.Errorf("unmarshal event: %w", err)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
