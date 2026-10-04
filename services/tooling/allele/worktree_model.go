package main

import (
	"encoding/json"
	"fmt"
	"time"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"

	"github.com/uptrace/bun"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// worktreeRow is Worktree's bun-mapped row. OwnerKind/Status are the proto enums' own int32 wire
// values -- simplest correct mapping, and this repo has no established jsonb-vs-int convention for
// a lone enum column to depart from (Bench/Relay's own jsonb columns are all for *nested message*
// fields, not bare enums).
type worktreeRow struct {
	bun.BaseModel `bun:"table:worktrees,alias:w"`

	// RepositoryID+Branch share one composite unique constraint (bun's "unique:<name>" convention --
	// matching services/tooling/foundry/model.go's own plugins_name_version precedent), not a bare
	// unique on Branch alone. Found live in Phase 9: a bare per-column unique let "wt-01" in one
	// repository collide with "wt-01" in a completely different one -- branch names are only ever
	// meant to be unique *within* a repository, the same way two different git remotes can obviously
	// both have their own "main".
	ID           string    `bun:"id,pk"`
	RepositoryID string    `bun:"repository_id,unique:worktrees_repository_id_branch"`
	Branch       string    `bun:"branch,unique:worktrees_repository_id_branch"`
	BaseCommit   string    `bun:"base_commit"`
	OwnerKind    int32     `bun:"owner_kind"`
	OwnerID      string    `bun:"owner_id"`
	TaskLabel    string    `bun:"task_label"`
	SymbolCount  int64     `bun:"symbol_count"`
	Status       int32     `bun:"status"`
	OpenedAt     time.Time `bun:"opened_at"`
}

func (w *worktreeRow) toProto() *allelev1.Worktree {
	return &allelev1.Worktree{
		Id:           w.ID,
		RepositoryId: w.RepositoryID,
		Branch:       w.Branch,
		BaseCommit:   w.BaseCommit,
		OwnerKind:    allelev1.WorktreeOwnerKind(w.OwnerKind),
		OwnerId:      w.OwnerID,
		TaskLabel:    w.TaskLabel,
		SymbolCount:  w.SymbolCount,
		Status:       allelev1.WorktreeStatus(w.Status),
		OpenedAt:     timestamppb.New(w.OpenedAt),
	}
}

// mergeQueueEntryRow is MergeQueueEntry's bun-mapped row. Checks is jsonb (protojson-encoded
// []*allelev1.VerificationStep) following the already-documented json.RawMessage-not-[]byte
// precedent (Relay/Bench's own Phase notes) for exactly this shape: a repeated proto message field
// with no meaningful relational structure of its own.
type mergeQueueEntryRow struct {
	bun.BaseModel `bun:"table:merge_queue_entries,alias:q"`

	ID           string          `bun:"id,pk"`
	Position     int32           `bun:"position"`
	ChangeID     string          `bun:"change_id"` // == the worktree's own id; see "Change is computed, not persisted" in store.go
	RepositoryID string          `bun:"repository_id"`
	Status       int32           `bun:"status"`
	Checks       json.RawMessage `bun:"checks,type:jsonb"`
	// BenchRunID is set when this entry's verification included a real Bench-triggered run (Phase
	// 10) -- internal bookkeeping only (no proto field exposes it directly; MergeQueueEntry's own
	// Checks already carries that run's outcome as an ordinary VerificationStep once it finishes).
	// Also what makes startup recovery possible: an entry still Status=RUNNING with this set when
	// Allele starts means its own background poller died with the previous process (watch-mode
	// restarts are frequent in this dev stack) -- see main.go's recoverPendingBenchRuns, which
	// re-launches a poller for exactly these rows instead of leaving them stuck mid-verification
	// forever.
	BenchRunID string    `bun:"bench_run_id"`
	Held       bool      `bun:"held"`
	QueuedAt   time.Time `bun:"queued_at"`
}

func marshalChecks(steps []*allelev1.VerificationStep) (json.RawMessage, error) {
	wrapper := &allelev1.VerificationRun{Steps: steps}
	b, err := protojson.Marshal(wrapper)
	if err != nil {
		return nil, fmt.Errorf("marshaling verification steps: %w", err)
	}
	return b, nil
}

func unmarshalChecks(raw json.RawMessage) ([]*allelev1.VerificationStep, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	wrapper := &allelev1.VerificationRun{}
	if err := protojson.Unmarshal(raw, wrapper); err != nil {
		return nil, fmt.Errorf("unmarshaling verification steps: %w", err)
	}
	return wrapper.GetSteps(), nil
}

func (q *mergeQueueEntryRow) toProto() (*allelev1.MergeQueueEntry, error) {
	checks, err := unmarshalChecks(q.Checks)
	if err != nil {
		return nil, err
	}
	return &allelev1.MergeQueueEntry{
		Id:           q.ID,
		Position:     q.Position,
		ChangeId:     q.ChangeID,
		RepositoryId: q.RepositoryID,
		Status:       allelev1.VerificationStatus(q.Status),
		Checks:       checks,
		Held:         q.Held,
		QueuedAt:     timestamppb.New(q.QueuedAt),
	}, nil
}
