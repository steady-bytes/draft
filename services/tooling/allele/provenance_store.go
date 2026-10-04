package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/steady-bytes/draft/services/tooling/allele/gitutil"

	"github.com/google/uuid"
)

var errPathPermissionNotFound = errors.New("path permission not found")
var errProvenanceNotFound = errors.New("provenance record not found")

// pathMatchesGlob reports whether path matches a glob pattern supporting "**" (any number of path
// segments, including crossing "/") and "*" (anything within a single segment) -- the shape every
// PathPermission.allow_globs/deny_globs entry uses (e.g. "fuse/balancer/**", matching the mockup's
// own examples). Implemented directly as a glob->regex translation rather than adding a dependency
// for this one, well-bounded piece of matching logic.
func pathMatchesGlob(glob, path string) bool {
	var re strings.Builder
	re.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch {
		case glob[i] == '*' && i+1 < len(glob) && glob[i+1] == '*':
			re.WriteString(".*")
			i++ // the loop's own i++ advances past the second '*' too
		case glob[i] == '*':
			re.WriteString("[^/]*")
		case strings.ContainsRune(`\.+()|[]{}^$`, rune(glob[i])):
			re.WriteString(`\`)
			re.WriteByte(glob[i])
		default:
			re.WriteByte(glob[i])
		}
	}
	re.WriteString("$")
	matched, err := regexp.MatchString(re.String(), path)
	return err == nil && matched
}

// evaluatePathPermissions decides whether identity may touch every path in paths, given repo's
// current PathPermission rows (R4.2). No rows at all for an identity means nothing has been
// granted yet -- denied by default, not an open door: this is an authorization boundary, and the
// safe default for an authorization boundary is deny, the same posture CheckPushPermission's own
// pre-receive-hook caller depends on to actually stop a push. A deny_globs match always wins over
// an allow_globs match on the same path, per the proto's own documented field semantics.
func evaluatePathPermissions(perms []*pathPermissionRow, identity string, paths []string) (allowed bool, reason string) {
	var applicable []*pathPermissionRow
	for _, p := range perms {
		if p.IdentityPattern == identity {
			applicable = append(applicable, p)
		}
	}
	if len(applicable) == 0 {
		return false, fmt.Sprintf("identity %q has no granted path permissions in this repository", identity)
	}

	for _, path := range paths {
		grantedForPath := false
		for _, p := range applicable {
			for _, g := range p.AllowGlobs {
				if pathMatchesGlob(g, path) {
					grantedForPath = true
				}
			}
		}
		for _, p := range applicable {
			for _, g := range p.DenyGlobs {
				if pathMatchesGlob(g, path) {
					grantedForPath = false // deny wins, even over another rule's allow
				}
			}
		}
		if !grantedForPath {
			return false, fmt.Sprintf("path %q is not covered by any granted permission for identity %q", path, identity)
		}
	}
	return true, ""
}

func (s *store) ListPathPermissions(ctx context.Context, repositoryID string) ([]*pathPermissionRow, error) {
	var rows []*pathPermissionRow
	q := s.db.NewSelect().Model(&rows)
	if repositoryID != "" {
		q = q.Where("repository_id = ?", repositoryID)
	}
	if err := q.OrderExpr("identity_pattern ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("listing path permissions: %w", err)
	}
	return rows, nil
}

// SetPathPermissions is a full replace for repositoryID's permission set, matching the proto's own
// documented "full replace, not incremental add" semantics for SetPathPermissionsRequest -- simpler
// to reason about than a merge, and it's what the mockup's own permissions editor does (the whole
// table is saved at once). Runs inside one transaction so a failed insert never leaves a repository
// with zero permissions it didn't ask for.
func (s *store) SetPathPermissions(ctx context.Context, repositoryID string, perms []*pathPermissionRow) ([]*pathPermissionRow, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.NewDelete().Model((*pathPermissionRow)(nil)).Where("repository_id = ?", repositoryID).Exec(ctx); err != nil {
		return nil, fmt.Errorf("clearing existing path permissions: %w", err)
	}

	out := make([]*pathPermissionRow, 0, len(perms))
	for _, p := range perms {
		row := &pathPermissionRow{
			ID:              uuid.NewString(),
			RepositoryID:    repositoryID,
			IdentityPattern: p.IdentityPattern,
			AllowGlobs:      p.AllowGlobs,
			DenyGlobs:       p.DenyGlobs,
		}
		if row.AllowGlobs == nil {
			row.AllowGlobs = []string{}
		}
		if row.DenyGlobs == nil {
			row.DenyGlobs = []string{}
		}
		if _, err := tx.NewInsert().Model(row).Exec(ctx); err != nil {
			return nil, fmt.Errorf("inserting path permission for %q: %w", p.IdentityPattern, err)
		}
		out = append(out, row)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing path permissions: %w", err)
	}
	return out, nil
}

// CheckPushPermission is the store-side half of the pre-receive hook's own permission check (see
// cmd/pre-receive-hook): load repositoryID's current rules fresh from Postgres (never cached -- a
// permission change must take effect on the very next push, not after some TTL) and evaluate them
// against the paths this push touches. Recording is a separate step (RecordProvenance, below),
// called by the RPC handler only when this returns allowed -- see that function's own doc comment
// for why a denied push leaves no row.
func (s *store) CheckPushPermission(ctx context.Context, repositoryID, identity string, paths []string) (allowed bool, reason string, err error) {
	perms, err := s.ListPathPermissions(ctx, repositoryID)
	if err != nil {
		return false, "", err
	}
	allowed, reason = evaluatePathPermissions(perms, identity, paths)
	return allowed, reason, nil
}

// ownerKindForRef looks up ref's existing worktree in repositoryID, if any, and returns its
// OwnerKind -- used to classify a recorded Provenance row without guessing. A brand-new branch's
// first push has no worktree row yet (see provenanceRow's own doc comment on the same ordering
// issue), so WORKTREE_OWNER_KIND_UNSPECIFIED is returned rather than a fabricated classification;
// its row still gets a real one on the branch's *next* push, once discovery has caught up.
func (s *store) ownerKindForRef(ctx context.Context, repositoryID, ref string) int32 {
	row := new(worktreeRow)
	err := s.db.NewSelect().Model(row).Where("repository_id = ? AND branch = ?", repositoryID, branchNameFromRef(ref)).Scan(ctx)
	if err != nil {
		return 0 // WORKTREE_OWNER_KIND_UNSPECIFIED
	}
	return row.OwnerKind
}

// branchNameFromRef strips git's "refs/heads/" prefix -- the hook passes on the full ref name
// exactly as it arrives on pre-receive's own stdin (e.g. "refs/heads/wt-01"), but worktreeRow.Branch
// stores short names only (gitutil.ListBranches's own "%(refname:short)" format). Found live: the
// very first ownerKindForRef lookup against a real pushed ref silently missed every row because of
// this mismatch, with no error to surface it (a lookup miss and "no worktree yet" look identical) --
// exactly the kind of bug that direct-RPC testing catches and a unit test against a mocked query
// would not have.
func branchNameFromRef(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

// RecordProvenance persists one Provenance row for an *allowed* push -- called by
// CheckPushPermission's RPC handler only on the allow path, never on denial. R4.1 asks for
// attribution of what actually entered the repository; a rejected push enters nothing, and its
// rejection is already surfaced synchronously to the pusher via the hook's own non-zero exit, so a
// denial leaves no additional row behind to query later.
func (s *store) RecordProvenance(ctx context.Context, commitSha, repositoryID, identity string, ownerKind int32, permittedPaths []string) error {
	if commitSha == "" {
		return nil // hook didn't supply one (e.g. a delete-ref push) -- nothing to key a row by
	}
	paths, err := json.Marshal(permittedPaths)
	if err != nil {
		return fmt.Errorf("marshaling permitted paths: %w", err)
	}
	row := &provenanceRow{
		CommitSha:          commitSha,
		RepositoryID:       repositoryID,
		Identity:           identity,
		OwnerKind:          ownerKind,
		PermittedPaths:     paths,
		PermissionViolated: false,
		RecordedAt:         time.Now().UTC(),
	}
	// The same commit can be re-evaluated (e.g. the hook runs again on a retried push that
	// resolves to the same sha) -- upsert on commit_sha rather than erroring on the second record.
	_, err = s.db.NewInsert().Model(row).
		On("CONFLICT (commit_sha) DO UPDATE").
		Set("identity = EXCLUDED.identity").
		Set("owner_kind = EXCLUDED.owner_kind").
		Set("permitted_paths = EXCLUDED.permitted_paths").
		Set("permission_violated = EXCLUDED.permission_violated").
		Set("recorded_at = EXCLUDED.recorded_at").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("recording provenance for commit %q: %w", commitSha, err)
	}
	return nil
}

func (s *store) GetProvenance(ctx context.Context, commitSha string) (*provenanceRow, error) {
	row := new(provenanceRow)
	err := s.db.NewSelect().Model(row).Where("commit_sha = ?", commitSha).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errProvenanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting provenance for commit %q: %w", commitSha, err)
	}
	return row, nil
}

// GetProvenanceByChangeID resolves changeID (a worktree id -- Change.Id and Worktree.ID are the
// same value; see worktree_store.go's computeChange) down to its branch's current HEAD commit, then
// looks up the provenance record for that exact commit. A worktree whose HEAD was never pushed
// through the pre-receive hook (e.g. a branch that existed before this phase's hook was installed)
// has no record -- returned as errProvenanceNotFound rather than a fabricated one.
func (s *store) GetProvenanceByChangeID(ctx context.Context, changeID string) (*provenanceRow, error) {
	wt, err := s.GetWorktree(ctx, changeID)
	if err != nil {
		return nil, err
	}
	repo, err := s.GetRepository(ctx, wt.RepositoryID)
	if err != nil {
		return nil, err
	}
	sha, err := gitutil.HeadCommit(s.repoPath(repo.Name), wt.Branch)
	if err != nil {
		return nil, fmt.Errorf("resolving HEAD for %s: %w", wt.Branch, err)
	}
	return s.GetProvenance(ctx, sha)
}
