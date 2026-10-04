package main

import (
	"encoding/json"
	"time"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"

	"github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// pathPermissionRow is PathPermission's bun-mapped row. AllowGlobs/DenyGlobs use bun's native
// Postgres array support (`,array`), the same convention repositoryRow.Languages already
// established in Phase 1 for a flat string list with no nested structure of its own.
type pathPermissionRow struct {
	bun.BaseModel `bun:"table:path_permissions,alias:pp"`

	ID              string   `bun:"id,pk"`
	RepositoryID    string   `bun:"repository_id"`
	IdentityPattern string   `bun:"identity_pattern"`
	AllowGlobs      []string `bun:"allow_globs,array"`
	DenyGlobs       []string `bun:"deny_globs,array"`
}

func (p *pathPermissionRow) toProto() *allelev1.PathPermission {
	return &allelev1.PathPermission{
		Id:              p.ID,
		RepositoryId:    p.RepositoryID,
		IdentityPattern: p.IdentityPattern,
		AllowGlobs:      p.AllowGlobs,
		DenyGlobs:       p.DenyGlobs,
	}
}

// provenanceRow is Provenance's bun-mapped row -- one per accepted push, written by
// CheckPushPermission's own RPC handler as a side effect of an allowed check (see
// provenance_store.go's RecordProvenance), not derived or recomputed later the way Change is. R4.1
// asks for this to be "queryable after the fact, not just logged at commit time and discarded"; a
// real table, not a log line, is what makes that true.
//
// Keyed by CommitSha, not a worktree/change id: the pre-receive hook that writes this runs *before*
// the ref is updated, so for a brand-new branch's very first push there is no worktree row yet to
// key against (store.go's syncWorktrees only discovers it afterwards, on the next sync) -- the
// pushed commit sha has no such ordering dependency, since the hook has it from its own stdin from
// the very first push onward. GetProvenance's own change_id parameter is resolved down to a commit
// sha at read time instead (see GetProvenanceByChangeID): a worktree's "current change" is just
// whatever commit its branch currently points to.
//
// SignatureKeyId stays empty in this phase -- Ed25519 server-side signing for agent/evolution-run
// worktrees and SSH-based verification for human ones (R4.3's other half) are a disclosed, deferred
// follow-up; see the plan's Decisions and this phase's own completion note.
type provenanceRow struct {
	bun.BaseModel `bun:"table:provenance,alias:prov"`

	CommitSha          string          `bun:"commit_sha,pk"`
	RepositoryID       string          `bun:"repository_id"`
	Identity           string          `bun:"identity"`
	OwnerKind          int32           `bun:"owner_kind"`
	ObjectiveID        string          `bun:"objective_id"`
	TaskID             string          `bun:"task_id"`
	PermittedPaths     json.RawMessage `bun:"permitted_paths,type:jsonb"` // []string, protojson-free (plain json.Marshal is fine for a string slice)
	PermissionViolated bool            `bun:"permission_violated"`
	SignatureKeyID     string          `bun:"signature_key_id"`
	RecordedAt         time.Time       `bun:"recorded_at"`
}

// toProto takes changeID explicitly since the row itself doesn't store one -- see the type's own
// doc comment on why Provenance is keyed by commit sha, not change id.
func (p *provenanceRow) toProto(changeID string) (*allelev1.Provenance, error) {
	var paths []string
	if len(p.PermittedPaths) > 0 {
		if err := json.Unmarshal(p.PermittedPaths, &paths); err != nil {
			return nil, err
		}
	}
	return &allelev1.Provenance{
		ChangeId:           changeID,
		Identity:           p.Identity,
		OwnerKind:          allelev1.WorktreeOwnerKind(p.OwnerKind),
		ObjectiveId:        p.ObjectiveID,
		TaskId:             p.TaskID,
		PermittedPaths:     paths,
		PermissionViolated: p.PermissionViolated,
		SignatureKeyId:     p.SignatureKeyID,
		CommitSha:          p.CommitSha,
		RecordedAt:         timestamppb.New(p.RecordedAt),
	}, nil
}
