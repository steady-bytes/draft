// This file is the persistence boundary the scheduler writes through: a small
// ResultStore interface, and pgResultStore, its real implementation on top of
// model.go's runRow/stepResultRow bun models. Kept as an interface (rather than the
// scheduler calling model.go's conversion functions directly) so scheduler_test.go
// can exercise the DAG/concurrency/retry/templating logic against an in-memory
// fake, without requiring a real Postgres instance for every test run — the same
// "boundary" discipline model.go and loader.go already establish elsewhere in this
// package.
//
// Per the brief: each step's result and status are persisted incrementally as they
// change (RUNNING when a step starts, its terminal status when it finishes) rather
// than only once at the end of the run, so a future GetRun/ListRuns (Phase 5) can
// observe in-progress state.
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	pgbun "github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
)

// Sentinel errors rpc.go/webhook.go map onto the right connect.Code /HTTP status —
// the same "distinguishable expected case, not a raw driver error" discipline
// services/tooling/garage/store.go's ErrNotFound/ErrAlreadyPublished establish for
// PluginCatalogService.
var (
	ErrRunNotFound            = errors.New("run not found")
	ErrWorkflowNotFound       = errors.New("workflow not found")
	ErrInvalidRunPageToken    = errors.New("invalid page token")
	ErrPluginRegistryNotFound = errors.New("plugin registry not found")
)

// defaultRunPageSize is used when ListRunsRequest.page_size is unset or
// non-positive — see services/tooling/garage/store.go's defaultListPageSize for the
// same convention.
const defaultRunPageSize = 50

// ResultStore is the persistence boundary the scheduler writes run/step state
// through.
type ResultStore interface {
	// UpsertRun writes run's top-level fields (not its Steps — those are written
	// individually via UpsertStepResult). Called once when a run starts (status
	// RUNNING) and once when it finishes (terminal status).
	UpsertRun(ctx context.Context, run *workflowv1.Run) error
	// UpsertStepResult writes one step's current result for runID. Called once
	// when the step starts (status RUNNING) and once when it reaches a terminal
	// status (passed/failed/skipped).
	UpsertStepResult(ctx context.Context, runID string, result *workflowv1.StepResult) error
}

// pgResultStore is ResultStore backed by Postgres, via the workflowRow/runRow/
// stepResultRow bun models in model.go.
type pgResultStore struct {
	db pgbun.Repository
}

// NewPostgresResultStore builds a store against an already-open bun repository (see
// model.go's createSchema for the tables it reads/writes). Returns the concrete
// *pgResultStore, not the narrower ResultStore interface: the scheduler only needs
// ResultStore's Upsert* methods (and *pgResultStore satisfies that interface
// implicitly), but rpc.go and webhook.go also need the Get*/List*/UpsertWorkflow
// methods below, which aren't part of ResultStore's contract — narrowing to an
// interface here would just mean re-widening it again at every other call site.
func NewPostgresResultStore(db pgbun.Repository) *pgResultStore {
	return &pgResultStore{db: db}
}

func (s *pgResultStore) UpsertRun(ctx context.Context, run *workflowv1.Run) error {
	row, _, err := runRowFromProto(run)
	if err != nil {
		return fmt.Errorf("failed to convert run %q for persistence: %w", run.GetRunId(), err)
	}

	_, err = s.db.Client().NewInsert().
		Model(row).
		On("CONFLICT (run_id) DO UPDATE").
		Set("workflow_name = EXCLUDED.workflow_name").
		Set("status = EXCLUDED.status").
		Set("started_at = EXCLUDED.started_at").
		Set("finished_at = EXCLUDED.finished_at").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to persist run %q: %w", run.GetRunId(), err)
	}
	return nil
}

func (s *pgResultStore) UpsertStepResult(ctx context.Context, runID string, result *workflowv1.StepResult) error {
	row, err := stepResultRowFromProto(runID, result)
	if err != nil {
		return fmt.Errorf("failed to convert step result %q for persistence: %w", result.GetStepName(), err)
	}

	_, err = s.db.Client().NewInsert().
		Model(row).
		On("CONFLICT (run_id, step_name) DO UPDATE").
		Set("status = EXCLUDED.status").
		Set("request = EXCLUDED.request").
		Set("result = EXCLUDED.result").
		Set("detail = EXCLUDED.detail").
		Set("error = EXCLUDED.error").
		Set("started_at = EXCLUDED.started_at").
		Set("finished_at = EXCLUDED.finished_at").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to persist step result %q for run %q: %w", result.GetStepName(), runID, err)
	}
	return nil
}

// GetRun fetches one run and its step results by run_id, reassembling them into a
// *workflowv1.Run via runRow.toProto — used by WorkflowService.GetRun (rpc.go) and
// the webhook handler's own status lookups. Returns ErrRunNotFound (not a raw
// sql.ErrNoRows) when run_id doesn't exist, so callers can map it onto
// connect.CodeNotFound / HTTP 404 without inspecting driver-level errors.
func (s *pgResultStore) GetRun(ctx context.Context, runID string) (*workflowv1.Run, error) {
	row := new(runRow)
	if err := s.db.Client().NewSelect().Model(row).Where("run_id = ?", runID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("failed to get run %q: %w", runID, err)
	}

	steps, err := s.stepResultsFor(ctx, runID)
	if err != nil {
		return nil, err
	}

	run, err := row.toProto(steps)
	if err != nil {
		return nil, fmt.Errorf("failed to convert run %q from storage: %w", runID, err)
	}
	return run, nil
}

// stepResultsFor fetches every stepResultRow for runID, ordered by started_at —
// the order steps actually began executing in, which for a DAG isn't necessarily
// declaration order but is the closest a flat list gets to it without also
// persisting the workflow's step index (out of scope for this phase; "simple is
// fine" per the brief).
func (s *pgResultStore) stepResultsFor(ctx context.Context, runID string) ([]*stepResultRow, error) {
	var steps []*stepResultRow
	if err := s.db.Client().NewSelect().
		Model(&steps).
		Where("run_id = ?", runID).
		OrderExpr("started_at ASC NULLS FIRST, step_name ASC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to get step results for run %q: %w", runID, err)
	}
	return steps, nil
}

// ListRuns returns runs ordered most-recent-first, optionally filtered to one
// workflow, keyset-paginated on (started_at, run_id) — the same cursor shape
// services/tooling/garage/store.go's list uses for (published_at, id), descending
// instead of ascending since "most recent first" is what a run history view wants.
func (s *pgResultStore) ListRuns(ctx context.Context, workflowName string, pageSize int32, pageToken string) ([]*workflowv1.Run, string, error) {
	cursor, err := decodeRunPageToken(pageToken)
	if err != nil {
		return nil, "", err
	}

	limit := int(pageSize)
	if limit <= 0 {
		limit = defaultRunPageSize
	}

	var rows []*runRow
	q := s.db.Client().NewSelect().
		Model(&rows).
		OrderExpr("started_at DESC, run_id DESC").
		Limit(limit + 1) // fetch one extra row to know whether there's a next page
	if workflowName != "" {
		q = q.Where("workflow_name = ?", workflowName)
	}
	if cursor != nil {
		q = q.Where("(started_at, run_id) < (?, ?)", cursor.startedAt, cursor.runID)
	}

	if err := q.Scan(ctx); err != nil {
		return nil, "", fmt.Errorf("failed to list runs: %w", err)
	}

	var nextPageToken string
	if len(rows) > limit {
		nextPageToken = encodeRunPageToken(rows[limit-1])
		rows = rows[:limit]
	}

	runs := make([]*workflowv1.Run, 0, len(rows))
	for _, row := range rows {
		steps, err := s.stepResultsFor(ctx, row.RunID)
		if err != nil {
			return nil, "", err
		}
		run, err := row.toProto(steps)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert run %q from storage: %w", row.RunID, err)
		}
		runs = append(runs, run)
	}
	return runs, nextPageToken, nil
}

// runPageCursor is the decoded form of a ListRunsResponse.next_page_token /
// ListRunsRequest.page_token: the (started_at, run_id) of the last row of the
// previous page, used as an exclusive upper bound (rows sort descending) for the
// next one.
type runPageCursor struct {
	startedAt time.Time
	runID     string
}

func encodeRunPageToken(row *runRow) string {
	startedAt := ""
	if row.StartedAt != nil {
		startedAt = row.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	raw := startedAt + "|" + row.RunID
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func decodeRunPageToken(token string) (*runPageCursor, error) {
	if token == "" {
		return nil, nil
	}

	raw, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRunPageToken, err)
	}

	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidRunPageToken
	}

	startedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRunPageToken, err)
	}

	return &runPageCursor{startedAt: startedAt, runID: parts[1]}, nil
}

// UpsertWorkflow persists w's definition, inserting a new row or updating the
// existing one for w.Name — a workflow file that changed on disk since the last
// restart (loadWorkflowsDir, called at startup from main.go) should update its
// stored definition, not fail on the workflows table's (name) primary key.
func (s *pgResultStore) UpsertWorkflow(ctx context.Context, w *workflowv1.Workflow) error {
	row, err := workflowRowFromProto(w)
	if err != nil {
		return fmt.Errorf("failed to convert workflow %q for persistence: %w", w.GetName(), err)
	}

	_, err = s.db.Client().NewInsert().
		Model(row).
		On("CONFLICT (name) DO UPDATE").
		Set("description = EXCLUDED.description").
		Set("definition = EXCLUDED.definition").
		Set("updated_at = current_timestamp").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to persist workflow %q: %w", w.GetName(), err)
	}
	return nil
}

// GetWorkflow fetches one workflow definition by name — used by TriggerRun and the
// webhook handler (after resolving a slug to a workflow name via the routing table
// main.go builds at startup) to load the actual *workflowv1.Workflow to run.
func (s *pgResultStore) GetWorkflow(ctx context.Context, name string) (*workflowv1.Workflow, error) {
	row := new(workflowRow)
	if err := s.db.Client().NewSelect().Model(row).Where("name = ?", name).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("failed to get workflow %q: %w", name, err)
	}
	w, err := row.toProto()
	if err != nil {
		return nil, fmt.Errorf("failed to convert workflow %q from storage: %w", name, err)
	}
	return w, nil
}

// ListWorkflows returns every workflow loaded from workflows_dir at startup
// (main.go's loadWorkflowsDir), ordered by name.
func (s *pgResultStore) ListWorkflows(ctx context.Context) ([]*workflowv1.Workflow, error) {
	var rows []*workflowRow
	if err := s.db.Client().NewSelect().Model(&rows).OrderExpr("name ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	workflows := make([]*workflowv1.Workflow, 0, len(rows))
	for _, row := range rows {
		w, err := row.toProto()
		if err != nil {
			return nil, fmt.Errorf("failed to convert workflow %q from storage: %w", row.Name, err)
		}
		workflows = append(workflows, w)
	}
	return workflows, nil
}

// DeleteWorkflow removes a workflow's definition by name. Its run history is
// untouched: runRow.WorkflowName is a plain string column, not a foreign
// key into workflows, so deleting the definition here doesn't cascade —
// deliberate, per the design doc: a workflow's past runs stay visible even
// after its definition is removed. Returns ErrWorkflowNotFound if name
// doesn't exist, matching GetWorkflow's own convention.
func (s *pgResultStore) DeleteWorkflow(ctx context.Context, name string) error {
	res, err := s.db.Client().NewDelete().Model((*workflowRow)(nil)).Where("name = ?", name).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete workflow %q: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine whether workflow %q was deleted: %w", name, err)
	}
	if affected == 0 {
		return ErrWorkflowNotFound
	}
	return nil
}

// ListPluginRegistries returns every configured plugin registry, ordered by
// created_at -- the order garage_plugin.go's federated resolution checks
// them in (oldest/first-added first, see the design doc's "Federated
// search, first-match execution").
func (s *pgResultStore) ListPluginRegistries(ctx context.Context) ([]*settingsv1.PluginRegistry, error) {
	var rows []*pluginRegistryRow
	if err := s.db.Client().NewSelect().Model(&rows).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list plugin registries: %w", err)
	}
	registries := make([]*settingsv1.PluginRegistry, 0, len(rows))
	for _, row := range rows {
		registries = append(registries, row.toProto())
	}
	return registries, nil
}

// GetPluginRegistry fetches one registry by name. Returns
// ErrPluginRegistryNotFound if it doesn't exist, matching GetWorkflow's
// convention.
func (s *pgResultStore) GetPluginRegistry(ctx context.Context, name string) (*settingsv1.PluginRegistry, error) {
	row := new(pluginRegistryRow)
	if err := s.db.Client().NewSelect().Model(row).Where("name = ?", name).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPluginRegistryNotFound
		}
		return nil, fmt.Errorf("failed to get plugin registry %q: %w", name, err)
	}
	return row.toProto(), nil
}

// UpsertPluginRegistry persists r's address, inserting a new row or
// replacing the existing one for r.Name -- unconditional, the same shape as
// UpsertWorkflow. addPluginRegistry/updatePluginRegistry (registry_write.go)
// enforce real create/update semantics on top of this, the same split
// createWorkflow/updateWorkflow already establish over UpsertWorkflow.
func (s *pgResultStore) UpsertPluginRegistry(ctx context.Context, r *settingsv1.PluginRegistry) error {
	row := pluginRegistryRowFromProto(r)
	_, err := s.db.Client().NewInsert().
		Model(row).
		On("CONFLICT (name) DO UPDATE").
		Set("address = EXCLUDED.address").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to persist plugin registry %q: %w", r.GetName(), err)
	}
	return nil
}

// DeletePluginRegistry removes a registry by name. Returns
// ErrPluginRegistryNotFound if it doesn't exist, matching DeleteWorkflow's
// convention.
func (s *pgResultStore) DeletePluginRegistry(ctx context.Context, name string) error {
	res, err := s.db.Client().NewDelete().Model((*pluginRegistryRow)(nil)).Where("name = ?", name).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete plugin registry %q: %w", name, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine whether plugin registry %q was deleted: %w", name, err)
	}
	if affected == 0 {
		return ErrPluginRegistryNotFound
	}
	return nil
}
