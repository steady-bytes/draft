package main

import (
	"time"

	"github.com/uptrace/bun"
)

// linkageApprovalRow records that a human has signed off on change_id's (a worktree id -- the same
// "Change" convention as everywhere else in this plan) own breaking linkage changes -- ApproveLinkageChange's
// own persisted effect. A row's mere existence means "approved"; there is no separate boolean column
// to go stale relative to it.
//
// Not yet consulted anywhere: EnqueueMerge doesn't check GetLinkageManifest/Diff/IsBreaking or this
// table before deciding PASSED/FAILED (see this phase's own completion note) -- ApproveLinkageChange
// is a real, working, persisted RPC today, but "a breaking change blocks the merge queue until
// approved" is a disclosed, not-yet-wired next increment, not silently claimed done.
type linkageApprovalRow struct {
	bun.BaseModel `bun:"table:linkage_approvals,alias:la"`

	RepositoryID string    `bun:"repository_id,pk"`
	ChangeID     string    `bun:"change_id,pk"`
	ApprovedAt   time.Time `bun:"approved_at"`
}
