// This file is the read side the UI adds to store.go: queries that answer "how have these
// workflows been doing?" without loading every run's step results. ListRuns (store.go) rebuilds
// whole Run messages, one extra query per run — right for GetRun/ListRuns RPCs, too heavy for an
// overview that needs the last dozen runs of every workflow.
package main

import (
	"context"
	"fmt"
	"time"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"github.com/uptrace/bun"
)

// runRecord is a run without its step results.
type runRecord struct {
	RunID    string
	Workflow string
	Status   workflowv1.RunStatus
	Started  time.Time // zero when the run has not started
	Finished time.Time // zero while the run is in flight
}

func newRunRecord(row *runRow) runRecord {
	rec := runRecord{
		RunID:    row.RunID,
		Workflow: row.WorkflowName,
		Status:   workflowv1.RunStatus(workflowv1.RunStatus_value[row.Status]),
	}
	if row.StartedAt != nil {
		rec.Started = row.StartedAt.UTC()
	}
	if row.FinishedAt != nil {
		rec.Finished = row.FinishedAt.UTC()
	}
	return rec
}

// InFlight reports whether the run has not reached a terminal status.
func (r runRecord) InFlight() bool {
	return r.Status == workflowv1.RunStatus_RUN_STATUS_RUNNING || r.Status == workflowv1.RunStatus_RUN_STATUS_PENDING
}

// Passed and Failed are the two terminal outcomes.
func (r runRecord) Passed() bool { return r.Status == workflowv1.RunStatus_RUN_STATUS_PASSED }
func (r runRecord) Failed() bool { return r.Status == workflowv1.RunStatus_RUN_STATUS_FAILED }

// Duration is how long a finished run took; ok is false while it is in flight or has no timestamps.
func (r runRecord) Duration() (d time.Duration, ok bool) {
	if r.Started.IsZero() || r.Finished.IsZero() {
		return 0, false
	}
	d = r.Finished.Sub(r.Started)
	if d < 0 {
		d = 0
	}
	return d, true
}

// runFilter narrows RunRecords.
type runFilter struct {
	Workflow string
	// Status is "", "passed", "failed" or "running" (in flight).
	Status string
	Since  time.Time
}

// statusValues maps a runFilter.Status onto the RunStatus names the runs table stores.
func statusValues(status string) []string {
	switch status {
	case "passed":
		return []string{workflowv1.RunStatus_RUN_STATUS_PASSED.String()}
	case "failed":
		return []string{workflowv1.RunStatus_RUN_STATUS_FAILED.String()}
	case "running":
		return []string{workflowv1.RunStatus_RUN_STATUS_RUNNING.String(), workflowv1.RunStatus_RUN_STATUS_PENDING.String()}
	}
	return nil
}

// RunRecords returns runs newest first, keyset-paginated like ListRuns (same page tokens), with
// no step results.
func (s *pgResultStore) RunRecords(ctx context.Context, f runFilter, pageSize int, pageToken string) ([]runRecord, string, error) {
	cursor, err := decodeRunPageToken(pageToken)
	if err != nil {
		return nil, "", err
	}
	limit := pageSize
	if limit <= 0 {
		limit = defaultRunPageSize
	}

	var rows []*runRow
	q := s.db.Client().NewSelect().
		Model(&rows).
		OrderExpr("started_at DESC, run_id DESC").
		Limit(limit + 1)
	if f.Workflow != "" {
		q = q.Where("workflow_name = ?", f.Workflow)
	}
	if vals := statusValues(f.Status); vals != nil {
		q = q.Where("status IN (?)", bun.In(vals))
	}
	if !f.Since.IsZero() {
		q = q.Where("started_at >= ?", f.Since)
	}
	if cursor != nil {
		q = q.Where("(started_at, run_id) < (?, ?)", cursor.startedAt, cursor.runID)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, "", fmt.Errorf("failed to list runs: %w", err)
	}

	var next string
	if len(rows) > limit {
		next = encodeRunPageToken(rows[limit-1])
		rows = rows[:limit]
	}
	out := make([]runRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, newRunRecord(row))
	}
	return out, next, nil
}

// LastRunRecords returns, for every workflow that has run, its last n runs newest first.
func (s *pgResultStore) LastRunRecords(ctx context.Context, n int) (map[string][]runRecord, error) {
	var rows []*runRow
	err := s.db.Client().NewSelect().
		TableExpr("(SELECT run_id, workflow_name, status, started_at, finished_at, "+
			"row_number() OVER (PARTITION BY workflow_name ORDER BY started_at DESC NULLS LAST, run_id DESC) AS rn FROM runs) AS r").
		ColumnExpr("r.run_id, r.workflow_name, r.status, r.started_at, r.finished_at").
		Where("r.rn <= ?", n).
		OrderExpr("r.workflow_name, r.started_at DESC NULLS LAST, r.run_id DESC").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("failed to load recent runs per workflow: %w", err)
	}
	out := make(map[string][]runRecord)
	for _, row := range rows {
		out[row.WorkflowName] = append(out[row.WorkflowName], newRunRecord(row))
	}
	return out, nil
}

// runCounts is how many runs started since some time, by outcome.
type runCounts struct {
	Total, Passed, Failed, InFlight int
}

// RunCounts counts runs that started at or after since.
func (s *pgResultStore) RunCounts(ctx context.Context, since time.Time) (runCounts, error) {
	var rows []struct {
		Status string `bun:"status"`
		N      int    `bun:"n"`
	}
	err := s.db.Client().NewSelect().
		TableExpr("runs").
		ColumnExpr("status, count(*) AS n").
		Where("started_at >= ?", since).
		GroupExpr("status").
		Scan(ctx, &rows)
	if err != nil {
		return runCounts{}, fmt.Errorf("failed to count runs: %w", err)
	}
	var c runCounts
	for _, r := range rows {
		c.Total += r.N
		switch workflowv1.RunStatus(workflowv1.RunStatus_value[r.Status]) {
		case workflowv1.RunStatus_RUN_STATUS_PASSED:
			c.Passed += r.N
		case workflowv1.RunStatus_RUN_STATUS_FAILED:
			c.Failed += r.N
		case workflowv1.RunStatus_RUN_STATUS_RUNNING, workflowv1.RunStatus_RUN_STATUS_PENDING:
			c.InFlight += r.N
		}
	}
	return c, nil
}

// FailedSteps names the step that failed in each of runIDs (the first one, if several did). A run
// with no failed step is absent from the result.
func (s *pgResultStore) FailedSteps(ctx context.Context, runIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(runIDs))
	if len(runIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		RunID    string `bun:"run_id"`
		StepName string `bun:"step_name"`
	}
	err := s.db.Client().NewSelect().
		TableExpr("step_results").
		ColumnExpr("run_id, step_name").
		Where("run_id IN (?)", bun.In(runIDs)).
		Where("status = ?", workflowv1.StepStatus_STEP_STATUS_FAILED.String()).
		OrderExpr("started_at ASC NULLS LAST, step_name ASC").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("failed to find failed steps: %w", err)
	}
	for _, r := range rows {
		if _, seen := out[r.RunID]; !seen {
			out[r.RunID] = r.StepName
		}
	}
	return out, nil
}
