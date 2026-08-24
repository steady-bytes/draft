package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	pgbun "github.com/steady-bytes/draft/pkg/repositories/postgres/bun"

	"github.com/uptrace/bun/driver/pgdriver"
)

// This file is the persistence layer behind PluginCatalogService's RPCs
// (rpc.go): translating catalog operations (publish/retract/get/list/search)
// into bun queries against the plugins table, and translating Postgres-level
// failures (a unique constraint violation, zero rows) into sentinel errors
// rpc.go can map onto the right connect.Code, rather than leaking driver
// errors up to callers.

var (
	// ErrAlreadyPublished is returned by store.publish when a row already
	// exists for the given (name, version) pair — the plugins_name_version
	// unique constraint on pluginRow (see model.go) rejected the insert.
	ErrAlreadyPublished = errors.New("plugin already published at this name and version")
	// ErrNotFound is returned by store.get when no row matches the given
	// (name, version) pair.
	ErrNotFound = errors.New("plugin not found")
	// ErrInvalidPageToken is returned by store.list when page_token doesn't
	// decode to a valid cursor.
	ErrInvalidPageToken = errors.New("invalid page token")
)

// defaultListPageSize is used when ListPluginsRequest.page_size is unset or
// non-positive. The catalog is expected to stay small (see the Garage
// design doc's Decided section on scale), so this is generous rather than
// tuned.
const defaultListPageSize = 50

type store struct {
	db pgbun.Repository
}

func newStore(db pgbun.Repository) *store {
	return &store{db: db}
}

// publish inserts a new row for the given plugin manifest. A duplicate
// (name, version) surfaces as ErrAlreadyPublished rather than the raw
// Postgres constraint-violation error, since it's an expected, distinguishable
// case (a publish step re-run, or two versions racing) rather than an
// internal failure.
func (s *store) publish(ctx context.Context, p *plugincatalogv1.Plugin) (*pluginRow, error) {
	row, err := newPluginRow(p)
	if err != nil {
		return nil, fmt.Errorf("failed to build row for plugin %s@%s: %w", p.GetName(), p.GetVersion(), err)
	}

	if _, err := s.db.Client().NewInsert().Model(row).Exec(ctx); err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: %s@%s", ErrAlreadyPublished, row.Name, row.Version)
		}
		return nil, fmt.Errorf("failed to insert plugin %s@%s: %w", row.Name, row.Version, err)
	}
	return row, nil
}

// retract deletes the row matching (name, version), if any. It reports
// whether a row was actually deleted, but callers (rpc.go's Retract) treat
// "no row matched" as a no-op, not an error — see rpc.go's Retract for the
// rationale.
func (s *store) retract(ctx context.Context, name, version string) (bool, error) {
	res, err := s.db.Client().NewDelete().
		Model((*pluginRow)(nil)).
		Where("name = ?", name).
		Where("version = ?", version).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to retract plugin %s@%s: %w", name, version, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to determine rows affected retracting %s@%s: %w", name, version, err)
	}
	return n > 0, nil
}

// get looks up the exact (name, version) row. Per the design doc's Open
// questions, Garage only ever resolves an exact pin — no version-range
// matching.
func (s *store) get(ctx context.Context, name, version string) (*pluginRow, error) {
	row := new(pluginRow)
	err := s.db.Client().NewSelect().
		Model(row).
		Where("name = ?", name).
		Where("version = ?", version).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get plugin %s@%s: %w", name, version, err)
	}
	return row, nil
}

// list returns published plugins ordered by (published_at, id), keyset-paginated
// on that pair. A simple offset-free cursor is enough for a table expected to
// stay small for a long time (see the design doc's Decided/Implementation
// plan notes on Garage's scale); it avoids the "page shifts under you while
// new plugins are published" problem an OFFSET-based scheme would have.
func (s *store) list(ctx context.Context, pageSize int32, pageToken string) (rows []*pluginRow, nextPageToken string, err error) {
	cursor, err := decodePageToken(pageToken)
	if err != nil {
		return nil, "", err
	}

	limit := int(pageSize)
	if limit <= 0 {
		limit = defaultListPageSize
	}

	q := s.db.Client().NewSelect().
		Model(&rows).
		OrderExpr("published_at ASC, id ASC").
		Limit(limit + 1) // fetch one extra row to know whether there's a next page
	if cursor != nil {
		q = q.Where("(published_at, id) > (?, ?)", cursor.publishedAt, cursor.id)
	}

	if err := q.Scan(ctx); err != nil {
		return nil, "", fmt.Errorf("failed to list plugins: %w", err)
	}

	if len(rows) > limit {
		nextPageToken = encodePageToken(rows[limit-1])
		rows = rows[:limit]
	}
	return rows, nextPageToken, nil
}

// search matches query against name and description via a simple ILIKE, per
// the design doc's Phase 2 deliverable ("a simple ILIKE ... is entirely
// sufficient"). An empty query returns every published plugin, ordered the
// same way list does.
func (s *store) search(ctx context.Context, query string) ([]*pluginRow, error) {
	var rows []*pluginRow
	q := s.db.Client().NewSelect().
		Model(&rows).
		OrderExpr("published_at ASC, id ASC")

	if query = strings.TrimSpace(query); query != "" {
		pattern := "%" + query + "%"
		q = q.Where("name ILIKE ?", pattern).WhereOr("description ILIKE ?", pattern)
	}

	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to search plugins: %w", err)
	}
	return rows, nil
}

// listVersions returns every published row for a plugin name, newest first
// (by published_at). This backs the UI's plugin detail page (Phase 4): "all
// versions of this name" isn't a query list/search/get already answer (list
// paginates across every name; get requires an exact version), so it's added
// here rather than reimplemented ad hoc in the UI layer.
func (s *store) listVersions(ctx context.Context, name string) ([]*pluginRow, error) {
	var rows []*pluginRow
	err := s.db.Client().NewSelect().
		Model(&rows).
		Where("name = ?", name).
		OrderExpr("published_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions for plugin %s: %w", name, err)
	}
	return rows, nil
}

// isUniqueViolation reports whether err is a Postgres unique_violation
// (SQLSTATE 23505) — pgdriver.Error carries the SQLSTATE code as the 'C'
// protocol field. This is how store.publish tells "duplicate (name,
// version)" apart from any other insert failure.
func isUniqueViolation(err error) bool {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23505"
	}
	return false
}

// pageCursor is the decoded form of a ListPluginsResponse.next_page_token /
// ListPluginsRequest.page_token: the (published_at, id) of the last row of
// the previous page, used as an exclusive lower bound for the next one.
type pageCursor struct {
	publishedAt time.Time
	id          string
}

// encodePageToken serializes row's (published_at, id) as an opaque token.
func encodePageToken(row *pluginRow) string {
	raw := row.PublishedAt.UTC().Format(time.RFC3339Nano) + "|" + row.ID
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// decodePageToken parses a token produced by encodePageToken. An empty token
// (the first page) decodes to a nil cursor, not an error.
func decodePageToken(token string) (*pageCursor, error) {
	if token == "" {
		return nil, nil
	}

	raw, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPageToken, err)
	}

	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidPageToken
	}

	publishedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPageToken, err)
	}

	return &pageCursor{publishedAt: publishedAt, id: parts[1]}, nil
}
