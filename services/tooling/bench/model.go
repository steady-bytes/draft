package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	pgbun "github.com/steady-bytes/draft/pkg/repositories/postgres/bun"

	"github.com/uptrace/bun"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file defines Bench's own storage shape for workflows/runs/step_results and
// the conversion functions between it and the api/tooling/workflow/v1 wire types.
//
// workflowv1.Workflow and workflowv1.Run are not flat: they carry nested messages
// (Trigger, repeated Step), repeated StepResult, and google.protobuf.Struct fields
// (arbitrary JSON) that don't map cleanly onto a single relational row the way bun
// expects a model to. Rather than fight that shape, the complex/nested parts are
// stored as jsonb and the boundary between "wire type" and "storage row" is made
// explicit via toProto()/*RowFromProto() below — see services/examples/crud/service/
// model.go for the contrasting case where a generated proto type (crudv1.Name) is
// flat enough to be used directly as a bun model, which Workflow/Run are not.

// workflowRow is Bench's storage shape for a Workflow definition. name/description
// are pulled out as their own columns for querying without unmarshaling; the rest of
// the definition (trigger, steps, and their nested with/expect structs) is stored
// verbatim as protobuf-canonical JSON in the definition column.
type workflowRow struct {
	bun.BaseModel `bun:"table:workflows,alias:w"`

	Name        string          `bun:"name,pk"`
	Description string          `bun:"description,notnull"`
	Definition  json.RawMessage `bun:"definition,type:jsonb,notnull"`
	CreatedAt   time.Time       `bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt   time.Time       `bun:"updated_at,nullzero,notnull,default:current_timestamp"`
}

// workflowRowFromProto converts a workflowv1.Workflow into its storage row. The
// full message (including the parts not broken out into their own columns) is
// preserved in Definition, so toProto is a lossless round trip.
//
// Definition (and stepResultRow.Result, below) are declared as json.RawMessage
// rather than []byte, even though protojson.Marshal returns a plain []byte:
// verified against a live Postgres instance while proving Phase 3 end to end that
// this distinction is load-bearing, not stylistic. bun's jsonb appender
// (schema.AppendJSONValue) re-marshals a field's value through encoding/json before
// writing it; encoding/json's own special-case for a bare []byte value is to
// base64-encode it as a JSON string (its documented behavior for arbitrary binary
// data), which silently double-encodes already-serialized JSON — protojson.Marshal's
// output stored in a plain []byte field round-trips through Postgres as a jsonb
// *string* containing base64, not the JSON object it actually is, breaking
// protojson.Unmarshal on read. json.RawMessage implements json.Marshaler to return
// its bytes verbatim, which is exactly "this is already JSON, embed it as-is" and
// is what bun's appender needs to do the right thing.
func workflowRowFromProto(w *workflowv1.Workflow) (*workflowRow, error) {
	definition, err := protojson.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal workflow %q: %w", w.GetName(), err)
	}
	return &workflowRow{
		Name:        w.GetName(),
		Description: w.GetDescription(),
		Definition:  json.RawMessage(definition),
	}, nil
}

func (r *workflowRow) toProto() (*workflowv1.Workflow, error) {
	w := &workflowv1.Workflow{}
	if err := protojson.Unmarshal(r.Definition, w); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow definition for %q: %w", r.Name, err)
	}
	return w, nil
}

// pluginRegistryRow is Bench's storage shape for one configured plugin
// registry (Phase 11's "Connecting a plugin registry" — see
// docs/website/content/docs/architecture/bench-workflow-engine.md). Flat
// enough (just a name and an address) that it needs no jsonb column at all,
// unlike workflowRow's Definition.
type pluginRegistryRow struct {
	bun.BaseModel `bun:"table:plugin_registries,alias:pr"`

	Name      string    `bun:"name,pk"`
	Address   string    `bun:"address,notnull"`
	CreatedAt time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp"`
}

func pluginRegistryRowFromProto(r *settingsv1.PluginRegistry) *pluginRegistryRow {
	return &pluginRegistryRow{Name: r.GetName(), Address: r.GetAddress()}
}

func (r *pluginRegistryRow) toProto() *settingsv1.PluginRegistry {
	return &settingsv1.PluginRegistry{Name: r.Name, Address: r.Address}
}

// runRow is Bench's storage shape for one Run of a Workflow. Unlike workflowRow,
// per-step detail is normalized into its own table (stepResultRow) rather than
// collapsed into a jsonb column here: "the last N runs of a workflow, with per-step
// drill-down" (the query pattern GetRun/ListRuns exist to serve, once Phase 5 wires
// them up) is exactly what a relational step_results table answers well and a jsonb
// blob doesn't.
type runRow struct {
	bun.BaseModel `bun:"table:runs,alias:r"`

	RunID        string `bun:"run_id,pk"`
	WorkflowName string `bun:"workflow_name,notnull"`
	// Status stores the RunStatus enum's string name (e.g. "RUN_STATUS_PASSED")
	// rather than its numeric value, so a row is human-readable without decoding
	// against the proto enum — see runRow.toProto/runRowFromProto for the mapping.
	Status     string     `bun:"status,notnull"`
	StartedAt  *time.Time `bun:"started_at"`
	FinishedAt *time.Time `bun:"finished_at"`
}

// stepResultRow is one step's outcome within a run, keyed by (run_id, step_name).
type stepResultRow struct {
	bun.BaseModel `bun:"table:step_results,alias:sr"`

	RunID    string `bun:"run_id,pk"`
	StepName string `bun:"step_name,pk"`
	Status   string `bun:"status,notnull"`
	// Request is the step's with: block after {{ steps.X.result }} templating
	// was resolved — what was actually sent, not the possibly-still-templated
	// with: from the workflow's YAML. Result is the executor's response
	// payload. Detail is optional executor-specific diagnostic info (e.g.
	// bench://grpc-call@v1's resolved address/URL/HTTP status). All three are
	// arbitrary google.protobuf.Struct, stored as protobuf-canonical JSON the
	// same way workflowRow.Definition stores a Workflow — json.RawMessage, not
	// []byte; see workflowRowFromProto's comment for why that distinction
	// matters here.
	Request    json.RawMessage `bun:"request,type:jsonb"`
	Result     json.RawMessage `bun:"result,type:jsonb"`
	Detail     json.RawMessage `bun:"detail,type:jsonb"`
	Error      string          `bun:"error,notnull"`
	StartedAt  *time.Time      `bun:"started_at"`
	FinishedAt *time.Time      `bun:"finished_at"`
}

// structToJSON marshals a google.protobuf.Struct to protobuf-canonical JSON
// for storage in a jsonb column, the same conversion workflowRowFromProto and
// stepResultRowFromProto already did inline for Result before Request/Detail
// existed too — pulled out once both needed the identical nil-safe logic.
func structToJSON(s *structpb.Struct, label string) (json.RawMessage, error) {
	if s == nil {
		return nil, nil
	}
	marshaled, err := protojson.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s: %w", label, err)
	}
	return json.RawMessage(marshaled), nil
}

// jsonToStruct reverses structToJSON. Empty input, or the literal 4-byte
// JSON value "null", both yield a nil Struct, not an error.
//
// The "null" case matters and is not redundant with the empty-input case:
// bun's jsonb appender re-marshals a Go value through encoding/json before
// writing it, and json.RawMessage(nil)'s documented MarshalJSON behavior is
// to emit exactly "null" — so a column that was never set round-trips
// through Postgres as the literal JSON value `null` (confirmed live via
// `column::text`), not as SQL NULL / a zero-length value. protojson.Unmarshal
// correctly rejects "null" as not a valid Struct (`{}` is empty-but-valid;
// `null` isn't a struct at all), so it has to be special-cased here the same
// way stepResultRow.Result's original toProto already did before Request/
// Detail existed — see that history for the fuller account of tracking this
// down against a live database.
func jsonToStruct(raw json.RawMessage, label string) (*structpb.Struct, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	s := &structpb.Struct{}
	if err := protojson.Unmarshal(trimmed, s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s: %w", label, err)
	}
	return s, nil
}

// runRowFromProto converts a workflowv1.Run into its run row plus one stepResultRow
// per entry in Run.steps, ready to be inserted into their respective tables.
func runRowFromProto(run *workflowv1.Run) (*runRow, []*stepResultRow, error) {
	row := &runRow{
		RunID:        run.GetRunId(),
		WorkflowName: run.GetWorkflowName(),
		Status:       run.GetStatus().String(),
		StartedAt:    timestampToTime(run.GetStartedAt()),
		FinishedAt:   timestampToTime(run.GetFinishedAt()),
	}

	steps := make([]*stepResultRow, 0, len(run.GetSteps()))
	for _, s := range run.GetSteps() {
		stepRow, err := stepResultRowFromProto(run.GetRunId(), s)
		if err != nil {
			return nil, nil, err
		}
		steps = append(steps, stepRow)
	}
	return row, steps, nil
}

func stepResultRowFromProto(runID string, s *workflowv1.StepResult) (*stepResultRow, error) {
	request, err := structToJSON(s.GetRequest(), fmt.Sprintf("request for step %q", s.GetStepName()))
	if err != nil {
		return nil, err
	}
	result, err := structToJSON(s.GetResult(), fmt.Sprintf("result for step %q", s.GetStepName()))
	if err != nil {
		return nil, err
	}
	detail, err := structToJSON(s.GetDetail(), fmt.Sprintf("detail for step %q", s.GetStepName()))
	if err != nil {
		return nil, err
	}
	return &stepResultRow{
		RunID:      runID,
		StepName:   s.GetStepName(),
		Status:     s.GetStatus().String(),
		Request:    request,
		Result:     result,
		Detail:     detail,
		Error:      s.GetError(),
		StartedAt:  timestampToTime(s.GetStartedAt()),
		FinishedAt: timestampToTime(s.GetFinishedAt()),
	}, nil
}

// toProto reassembles a runRow and its stepResultRows (fetched separately, since
// they live in their own table) back into a workflowv1.Run.
func (r *runRow) toProto(steps []*stepResultRow) (*workflowv1.Run, error) {
	stepResults := make([]*workflowv1.StepResult, 0, len(steps))
	for _, s := range steps {
		sr, err := s.toProto()
		if err != nil {
			return nil, err
		}
		stepResults = append(stepResults, sr)
	}
	return &workflowv1.Run{
		RunId:        r.RunID,
		WorkflowName: r.WorkflowName,
		Status:       workflowv1.RunStatus(workflowv1.RunStatus_value[r.Status]),
		StartedAt:    timeToTimestamp(r.StartedAt),
		FinishedAt:   timeToTimestamp(r.FinishedAt),
		Steps:        stepResults,
	}, nil
}

func (r *stepResultRow) toProto() (*workflowv1.StepResult, error) {
	// See jsonToStruct's doc comment for why "null"-vs-empty both have to be
	// treated as "nothing here" — the fact that they do is a real, previously
	// hard-won finding (Phase 3), not an assumption repeated here for the two
	// new columns.
	request, err := jsonToStruct(r.Request, fmt.Sprintf("request for step %q", r.StepName))
	if err != nil {
		return nil, err
	}
	result, err := jsonToStruct(r.Result, fmt.Sprintf("result for step %q", r.StepName))
	if err != nil {
		return nil, err
	}
	detail, err := jsonToStruct(r.Detail, fmt.Sprintf("detail for step %q", r.StepName))
	if err != nil {
		return nil, err
	}
	return &workflowv1.StepResult{
		StepName:   r.StepName,
		Status:     workflowv1.StepStatus(workflowv1.StepStatus_value[r.Status]),
		Request:    request,
		Result:     result,
		Detail:     detail,
		Error:      r.Error,
		StartedAt:  timeToTimestamp(r.StartedAt),
		FinishedAt: timeToTimestamp(r.FinishedAt),
	}, nil
}

func timestampToTime(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func timeToTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// createSchema creates the workflows/runs/step_results tables if they don't already
// exist. Not required for Phase 1 — nothing queries or writes to these tables yet —
// but it's a cheap way to prove the bun model mapping above is actually valid
// against a real Postgres instance, not just that it compiles. Called from main via
// WithRunner; failures are logged, not fatal, since a missing local Postgres
// shouldn't block the rest of Bench's startup at this phase.
func createSchema(ctx context.Context, db pgbun.Repository) error {
	models := []interface{}{
		(*workflowRow)(nil),
		(*runRow)(nil),
		(*stepResultRow)(nil),
		(*pluginRegistryRow)(nil),
	}
	for _, model := range models {
		if _, err := db.Client().NewCreateTable().Model(model).IfNotExists().Exec(ctx); err != nil {
			return fmt.Errorf("failed to create table for %T: %w", model, err)
		}
	}

	// request/detail were added to step_results after it first shipped, so an
	// already-existing table (any local dev Postgres from before this change)
	// won't pick them up from CREATE TABLE IF NOT EXISTS above. Postgres
	// supports ADD COLUMN IF NOT EXISTS natively, so this is safe to run on
	// every startup, whether the table is brand new or not.
	alterStmts := []string{
		"ALTER TABLE step_results ADD COLUMN IF NOT EXISTS request jsonb",
		"ALTER TABLE step_results ADD COLUMN IF NOT EXISTS detail jsonb",
	}
	for _, stmt := range alterStmts {
		if _, err := db.Client().ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to migrate step_results schema (%q): %w", stmt, err)
		}
	}

	return nil
}
