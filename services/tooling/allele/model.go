package main

import (
	"context"
	"fmt"
	"time"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	pgbun "github.com/steady-bytes/draft/pkg/repositories/postgres/bun"

	"github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// repositoryRow is Repository's bun-mapped row. A dedicated struct, not the generated proto type
// used directly as a bun model, since Repository carries a google.protobuf.Timestamp field
// (CreatedAt here is a plain time.Time) -- the same precedent Bench/Relay's own Phase 1 models
// already established (see docs/website/content/docs/architecture/allele-implementation-plan.md's
// "Postgres, not Blueprint KV" section).
//
// Languages uses bun's native Postgres array support (`,array` -- backed by pgdialect, already
// this service's driver) rather than a jsonb column -- this is the first place in this repo that
// stores a plain []string this way; every other repeated-string-shaped field elsewhere went into
// jsonb instead because it sat alongside other nested/structured data in the same column. A flat
// string list has no such need, and a real Postgres text[] is both simpler and queryable
// (`languages @> ARRAY['go']`) in a way jsonb isn't without extra casting.
type repositoryRow struct {
	bun.BaseModel `bun:"table:repositories,alias:r"`

	ID            string    `bun:"id,pk"`
	Name          string    `bun:"name,unique"`
	Description   string    `bun:"description"`
	Languages     []string  `bun:"languages,array"`
	DefaultBranch string    `bun:"default_branch"`
	SymbolCount   int64     `bun:"symbol_count"`
	CreatedAt     time.Time `bun:"created_at"`
	// BreakingChangePolicy (Phase 11, R5.4) -- not exposed on Repository's own proto message (it
	// surfaces only through LinkageManifest.policy, the linkage-specific RPCs' own read-model);
	// stored here because it's a per-repository setting with nowhere else to live. Defaults to
	// BREAKING_CHANGE_POLICY_INTERFACE_ONLY at creation -- the mockup's own pre-checked radio
	// option (see the plan's Decisions) -- not UNSPECIFIED, so a repository that never calls
	// SetBreakingChangePolicy still gets a real, intentional default rather than a zero value that
	// happens to disable the feature (UNSPECIFIED and NEVER would otherwise be indistinguishable to
	// a caller checking the policy, which is the wrong default for a safety-oriented setting).
	BreakingChangePolicy int32 `bun:"breaking_change_policy"`
}

func (r *repositoryRow) toProto() *allelev1.Repository {
	return &allelev1.Repository{
		Id:            r.ID,
		Name:          r.Name,
		Description:   r.Description,
		Languages:     r.Languages,
		DefaultBranch: r.DefaultBranch,
		SymbolCount:   r.SymbolCount,
		CreatedAt:     timestamppb.New(r.CreatedAt),
	}
}

func createSchema(ctx context.Context, db pgbun.Repository) error {
	models := []interface{}{
		(*repositoryRow)(nil),
		(*worktreeRow)(nil),
		(*mergeQueueEntryRow)(nil),
		(*pathPermissionRow)(nil),
		(*provenanceRow)(nil),
		(*linkageApprovalRow)(nil),
	}
	for _, m := range models {
		if _, err := db.Client().NewCreateTable().Model(m).IfNotExists().Exec(ctx); err != nil {
			return fmt.Errorf("failed to create table for %T: %w", m, err)
		}
	}
	return nil
}
