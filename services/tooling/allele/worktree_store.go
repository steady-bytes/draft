package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	"github.com/steady-bytes/draft/services/tooling/allele/diff"
	"github.com/steady-bytes/draft/services/tooling/allele/gitutil"
	"github.com/steady-bytes/draft/services/tooling/allele/merge"
	"github.com/steady-bytes/draft/services/tooling/allele/parsing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var errWorktreeNotFound = errors.New("worktree not found")

// syncWorktrees reconciles the worktrees table against repo's actual branches -- "opening one is
// observed, not commanded" (the plan's own RPC Interface section): there is no CreateWorktree RPC.
// A worktree's row is created the first time its branch is found to exist, discovered here by
// walking the bare repo's own refs, not pushed from a git hook. This is Phase 7's own, deliberately
// simpler mechanism; Phase 8's pre-receive hook work is for permission *enforcement* at push time,
// a different need than noticing a branch exists, and this plan's Decisions never committed Phase 7
// to needing a hook at all for that simpler job.
func (s *store) syncWorktrees(ctx context.Context, repo *repositoryRow) error {
	branches, err := gitutil.ListBranches(s.repoPath(repo.Name))
	if err != nil {
		return fmt.Errorf("listing branches for %s: %w", repo.Name, err)
	}
	branchSet := make(map[string]bool, len(branches))
	for _, b := range branches {
		branchSet[b] = true
	}

	var existing []*worktreeRow
	if err := s.db.NewSelect().Model(&existing).Where("repository_id = ?", repo.ID).Scan(ctx); err != nil {
		return fmt.Errorf("listing existing worktrees for %s: %w", repo.Name, err)
	}
	existingByBranch := make(map[string]*worktreeRow, len(existing))
	for _, w := range existing {
		existingByBranch[w.Branch] = w
	}

	for _, branch := range branches {
		if branch == repo.DefaultBranch {
			continue // the default branch itself is never a "worktree"
		}
		if _, known := existingByBranch[branch]; known {
			continue
		}
		if err := s.createWorktreeFromBranch(ctx, repo, branch); err != nil {
			return err
		}
	}

	// A worktree whose branch no longer exists was merged (fast-forwarded and deleted) or removed
	// since the last sync -- mark it MERGED rather than deleting the row, so it still shows in
	// whatever history view eventually wants it.
	for branch, w := range existingByBranch {
		if !branchSet[branch] && w.Status != int32(allelev1.WorktreeStatus_WORKTREE_STATUS_MERGED) {
			if _, err := s.db.NewUpdate().Model(w).Set("status = ?", int32(allelev1.WorktreeStatus_WORKTREE_STATUS_MERGED)).Where("id = ?", w.ID).Exec(ctx); err != nil {
				return fmt.Errorf("marking %s merged: %w", branch, err)
			}
		}
	}
	return nil
}

func (s *store) createWorktreeFromBranch(ctx context.Context, repo *repositoryRow, branch string) error {
	path := s.repoPath(repo.Name)
	base, err := gitutil.MergeBase(path, repo.DefaultBranch, branch)
	if err != nil {
		return fmt.Errorf("finding merge-base for %s: %w", branch, err)
	}
	_, email, err := gitutil.CommitAuthor(path, branch)
	if err != nil {
		return fmt.Errorf("reading commit author for %s: %w", branch, err)
	}

	row := &worktreeRow{
		ID:           uuid.NewString(),
		RepositoryID: repo.ID,
		Branch:       branch,
		BaseCommit:   base,
		// A commit-author-derived placeholder, not real server-enforced identity -- see
		// gitutil.CommitAuthor's own doc comment. Phase 8 replaces OwnerKind/OwnerID with the
		// X-Authentik-Username that arrives with the actual push.
		OwnerKind: int32(allelev1.WorktreeOwnerKind_WORKTREE_OWNER_KIND_HUMAN),
		OwnerID:   email,
		Status:    int32(allelev1.WorktreeStatus_WORKTREE_STATUS_READY),
		OpenedAt:  time.Now().UTC(),
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		return fmt.Errorf("inserting worktree row for %s: %w", branch, err)
	}
	return nil
}

// ListWorktrees syncs every repository named (or every repository, if repositoryID is empty --
// "empty matches every repository", the convention ListWorktreesRequest's own field comment
// states) against its real branches before listing, so a result is never stale relative to what
// git itself currently has.
func (s *store) ListWorktrees(ctx context.Context, repositoryID string) ([]*worktreeRow, error) {
	var repos []*repositoryRow
	rq := s.db.NewSelect().Model(&repos)
	if repositoryID != "" {
		rq = rq.Where("id = ?", repositoryID)
	}
	if err := rq.Scan(ctx); err != nil {
		return nil, fmt.Errorf("listing repositories to sync: %w", err)
	}
	for _, repo := range repos {
		if err := s.syncWorktrees(ctx, repo); err != nil {
			return nil, err
		}
	}

	var rows []*worktreeRow
	wq := s.db.NewSelect().Model(&rows)
	if repositoryID != "" {
		wq = wq.Where("repository_id = ?", repositoryID)
	}
	if err := wq.OrderExpr("opened_at ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}
	return rows, nil
}

func (s *store) GetWorktree(ctx context.Context, id string) (*worktreeRow, error) {
	row := new(worktreeRow)
	err := s.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errWorktreeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting worktree %q: %w", id, err)
	}
	return row, nil
}

// getWorktreeByBranch is NotifyPush's own lookup -- the post-receive hook knows a repository and a
// ref, never a worktree's database id (the same reason getRepositoryByName exists for name-addressed
// lookups elsewhere in this file).
func (s *store) getWorktreeByBranch(ctx context.Context, repositoryID, branch string) (*worktreeRow, error) {
	row := new(worktreeRow)
	err := s.db.NewSelect().Model(row).Where("repository_id = ? AND branch = ?", repositoryID, branch).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errWorktreeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting worktree for branch %q: %w", branch, err)
	}
	return row, nil
}

// NotifyPush is called by the post-receive hook once per accepted ref update (Phase 9's own
// Telemetry row: "tooling.allele.v1.SymbolChanged ... every symbol-level change indexed from a
// push"). Deliberately reuses computeChange rather than diffing old..new itself: computeChange
// already is "this worktree's current symbol-level diff against its base," exactly what changed by
// this push once the ref is at its new position, and keeping one diff path means Phase 6/7's
// reconciliation and conflict logic stay the only place that logic lives. A push straight to the
// repository's own default branch is not a worktree at all (see syncWorktrees's own doc comment) --
// nothing to index as a Change, so this is a deliberate no-op for it, not a missed case.
func (s *store) NotifyPush(ctx context.Context, repositoryID, ref string) error {
	repo, err := s.GetRepository(ctx, repositoryID)
	if err != nil {
		return err
	}
	branch := branchNameFromRef(ref)
	if branch == repo.DefaultBranch {
		return nil
	}
	if err := s.syncWorktrees(ctx, repo); err != nil {
		return err
	}
	wt, err := s.getWorktreeByBranch(ctx, repositoryID, branch)
	if err != nil {
		return err
	}
	change, err := s.computeChange(ctx, wt)
	if err != nil {
		return err
	}
	for _, sc := range change.Symbols {
		s.events.SymbolChanged(repositoryID, wt.ID, sc)
	}
	return nil
}

// CloseWorktree deletes the row outright -- "abandon, no merge" (the RPC's own doc comment) has no
// history worth keeping the way a real MERGED worktree does; WorktreeStatus has no distinct
// "abandoned" value, and adding one for a row about to disappear anyway isn't worth a proto change.
func (s *store) CloseWorktree(ctx context.Context, id string) error {
	res, err := s.db.NewDelete().Model((*worktreeRow)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("closing worktree %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errWorktreeNotFound
	}
	return nil
}

// worktreeOverlap computes merge.Overlap between two worktrees' branches, each against the
// repository's own default branch as their shared base -- an approximation of "their real
// merge-base" that's exactly right when neither has rebased onto the other, and the one every
// other open worktree in the overlap matrix needs simultaneously anyway (O(n) loads fed into an
// O(n²) comparison, matching the plan's own "isolated behind one function boundary" decision).
func (s *store) worktreeOverlap(repoPath, defaultBranch string, a, b *worktreeRow) (merge.OverlapResult, error) {
	// Branch name, not the worktree's own database id, as the "id" LoadSideSymbols stamps into
	// every ConflictReport's explanation text -- found live (see the plan's Phase 7 completion
	// note): an opaque UUID in "<id> removes the field..." reads far worse than "wt-04 removes the
	// field...", and a branch name is already a valid, unique-per-repo way to look a worktree back
	// up if a caller needs to. OverlapPair.worktree_id_a/b (below) still carry the real ids.
	aSide, err := merge.LoadSideSymbols(repoPath, defaultBranch, a.Branch, a.Branch)
	if err != nil {
		return merge.OverlapResult{}, err
	}
	bSide, err := merge.LoadSideSymbols(repoPath, defaultBranch, b.Branch, b.Branch)
	if err != nil {
		return merge.OverlapResult{}, err
	}
	return merge.Overlap(aSide, bSide)
}

// computeChange builds a real Change by diffing wt's branch against its repository's default
// branch, using Phase 5's diff.SymbolChanges per changed file and Phase 6's merge.Overlap against
// every other currently-open worktree for Conflicts. Change is deliberately NOT persisted: it's a
// live view of "this worktree's current diff," recomputed every call so it can never go stale
// relative to a new push the way a cached row could -- its own id is just the worktree's id, since
// an open worktree has exactly one current Change.
func (s *store) computeChange(ctx context.Context, wt *worktreeRow) (*allelev1.Change, error) {
	repo, err := s.GetRepository(ctx, wt.RepositoryID)
	if err != nil {
		return nil, err
	}
	path := s.repoPath(repo.Name)

	paths, err := gitutil.ChangedFiles(path, repo.DefaultBranch, wt.Branch)
	if err != nil {
		return nil, fmt.Errorf("listing changed files: %w", err)
	}

	title := wt.TaskLabel
	if title == "" {
		title = wt.Branch
	}
	change := &allelev1.Change{
		Id:           wt.ID,
		RepositoryId: wt.RepositoryID,
		WorktreeId:   wt.ID,
		Title:        title,
		OpenedAt:     timestamppb.New(wt.OpenedAt),
	}

	for _, p := range paths {
		lang := parsing.LanguageForPath(p)
		if lang == parsing.LanguageUnknown {
			continue
		}
		oldSrc, err := gitutil.ShowFile(path, repo.DefaultBranch, p)
		if err != nil {
			return nil, err
		}
		newSrc, err := gitutil.ShowFile(path, wt.Branch, p)
		if err != nil {
			return nil, err
		}
		symbolChanges, err := diff.SymbolChanges(lang, p, oldSrc, newSrc)
		if err != nil {
			return nil, fmt.Errorf("diffing %s: %w", p, err)
		}
		change.Symbols = append(change.Symbols, symbolChanges...)
		change.FilesChanged++
	}

	added, removed, err := gitutil.DiffStat(path, repo.DefaultBranch, wt.Branch)
	if err != nil {
		return nil, fmt.Errorf("computing diff stat: %w", err)
	}
	change.LinesAdded, change.LinesRemoved = added, removed

	others, err := s.ListWorktrees(ctx, wt.RepositoryID)
	if err != nil {
		return nil, err
	}
	for _, other := range others {
		if other.ID == wt.ID || other.Status == int32(allelev1.WorktreeStatus_WORKTREE_STATUS_MERGED) {
			continue
		}
		result, err := s.worktreeOverlap(path, repo.DefaultBranch, wt, other)
		if err != nil {
			return nil, fmt.Errorf("checking overlap against %s: %w", other.Branch, err)
		}
		change.Conflicts = append(change.Conflicts, result.Conflicts...)
	}

	return change, nil
}

// GetOverlapMatrix computes merge.Overlap for every pair of currently-open (non-MERGED) worktrees
// in a repository -- allele-worktrees.html's own centerpiece. O(n²) comparisons over O(n) loaded
// sides, isolated behind this one function per the plan's own Decisions, so it can become
// incremental later without any caller (this RPC handler, or EnqueueMerge/computeChange, which
// both call worktreeOverlap directly for a single pair) having to change.
func (s *store) GetOverlapMatrix(ctx context.Context, repositoryID string) ([]*allelev1.OverlapPair, error) {
	repo, err := s.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	worktrees, err := s.ListWorktrees(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	var open []*worktreeRow
	for _, w := range worktrees {
		if w.Status != int32(allelev1.WorktreeStatus_WORKTREE_STATUS_MERGED) {
			open = append(open, w)
		}
	}

	path := s.repoPath(repo.Name)
	var pairs []*allelev1.OverlapPair
	for i := 0; i < len(open); i++ {
		for j := i + 1; j < len(open); j++ {
			a, b := open[i], open[j]
			result, err := s.worktreeOverlap(path, repo.DefaultBranch, a, b)
			if err != nil {
				return nil, fmt.Errorf("computing overlap between %s and %s: %w", a.Branch, b.Branch, err)
			}

			kind := allelev1.OverlapKind_OVERLAP_KIND_NONE
			detail := ""
			switch {
			case len(result.Conflicts) > 0:
				kind = allelev1.OverlapKind_OVERLAP_KIND_SEMANTIC_CONFLICT
				detail = result.Conflicts[0].GetExplanation()
			case len(result.AutoMerged) > 0:
				kind = allelev1.OverlapKind_OVERLAP_KIND_SAME_FILES_DIFFERENT_SYMBOLS
				detail = fmt.Sprintf("%d symbol(s) reconciled cleanly", len(result.AutoMerged))
			case len(result.SharedFiles) > 0:
				kind = allelev1.OverlapKind_OVERLAP_KIND_SAME_FILES_DIFFERENT_SYMBOLS
				detail = fmt.Sprintf("same file %s; different symbols · auto-merge", result.SharedFiles[0])
			default:
				continue // no overlap at all -- the mockup's own matrix leaves these cells blank
			}
			pairs = append(pairs, &allelev1.OverlapPair{
				WorktreeIdA: a.ID,
				WorktreeIdB: b.ID,
				Kind:        kind,
				Detail:      detail,
			})
		}
	}
	return pairs, nil
}

// -- Merge queue ----------------------------------------------------------------------------------

// EnqueueMerge adds changeID (a worktree id -- see GetChange's own doc comment on why Change is
// computed, not persisted, so there is no separately-generated change id to key this by) to the
// repository's merge queue, running the AST overlap check (R2's own half of verification) against
// every other currently-open worktree right away, then -- if the AST check passed and the
// worktree's branch commits a .allele/verify.yaml -- triggering a real Bench-backed run for the
// other half (Phase 10; see verification.go). A repository with no verify.yaml gets the AST check
// alone, same as Phase 7 shipped; that alone is now a real PASSED, not an indefinite WAITING -- see
// below.
func (s *store) EnqueueMerge(ctx context.Context, worktreeID string) (*mergeQueueEntryRow, error) {
	wt, err := s.GetWorktree(ctx, worktreeID)
	if err != nil {
		return nil, err
	}
	repo, err := s.GetRepository(ctx, wt.RepositoryID)
	if err != nil {
		return nil, err
	}

	others, err := s.ListWorktrees(ctx, wt.RepositoryID)
	if err != nil {
		return nil, err
	}

	path := s.repoPath(repo.Name)
	astStep := &allelev1.VerificationStep{
		Name:   "Merge simulation with open worktrees",
		Status: allelev1.VerificationStatus_VERIFICATION_STATUS_PASSED,
	}
	for _, other := range others {
		if other.ID == wt.ID || other.Status == int32(allelev1.WorktreeStatus_WORKTREE_STATUS_MERGED) {
			continue
		}
		result, err := s.worktreeOverlap(path, repo.DefaultBranch, wt, other)
		if err != nil {
			return nil, fmt.Errorf("checking overlap against %s: %w", other.Branch, err)
		}
		if len(result.Conflicts) > 0 {
			astStep.Status = allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED
			astStep.Detail = result.Conflicts[0].GetExplanation()
			s.events.ConflictDetected(wt.RepositoryID, wt.ID, result.Conflicts[0])
			break
		}
	}

	steps := []*allelev1.VerificationStep{astStep}
	overallStatus := allelev1.VerificationStatus_VERIFICATION_STATUS_PASSED
	benchRunID := ""

	if astStep.Status == allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED {
		overallStatus = allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED
	} else if s.bench != nil {
		// gitutil.ShowFile returns (nil, nil) for a path that doesn't exist at ref -- a repository
		// that never opted in gets exactly Phase 7's own AST-only behavior, nothing more.
		verifyYAML, showErr := gitutil.ShowFile(path, wt.Branch, verifyYAMLPath)
		if showErr != nil {
			return nil, fmt.Errorf("reading %s from %s: %w", verifyYAMLPath, wt.Branch, showErr)
		}
		if verifyYAML != nil {
			benchStep, runID, benchErr := s.triggerBenchVerification(ctx, repo, wt, verifyYAML)
			steps = append(steps, benchStep)
			if benchErr != nil {
				overallStatus = allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED
			} else {
				overallStatus = allelev1.VerificationStatus_VERIFICATION_STATUS_RUNNING
				benchRunID = runID
			}
		}
	}

	checksJSON, err := marshalChecks(steps)
	if err != nil {
		return nil, err
	}

	var maxPos sql.NullInt32
	if err := s.db.NewSelect().Model((*mergeQueueEntryRow)(nil)).
		Where("repository_id = ?", wt.RepositoryID).
		ColumnExpr("MAX(position)").Scan(ctx, &maxPos); err != nil {
		return nil, fmt.Errorf("finding current queue depth: %w", err)
	}

	row := &mergeQueueEntryRow{
		ID:           uuid.NewString(),
		Position:     maxPos.Int32 + 1,
		ChangeID:     worktreeID,
		RepositoryID: wt.RepositoryID,
		Status:       int32(overallStatus),
		Checks:       checksJSON,
		BenchRunID:   benchRunID,
		QueuedAt:     time.Now().UTC(),
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, fmt.Errorf("inserting merge queue entry: %w", err)
	}

	if err := s.setWorktreeStatusForVerification(ctx, wt.ID, overallStatus); err != nil {
		return nil, err
	}

	switch overallStatus {
	case allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED, allelev1.VerificationStatus_VERIFICATION_STATUS_PASSED:
		// Both are real terminal states -- PASSED only became reachable with Phase 10 (a
		// repository with no verify.yaml, or whose AST check alone already failed, has nothing
		// further pending), so both fire VerificationCompleted; WAITING never occurs anymore (see
		// above), and RUNNING is deliberately excluded below -- the background poller fires this
		// once Bench's own run actually reaches one of these two statuses for real.
		now := timestamppb.Now()
		s.events.VerificationCompleted(wt.RepositoryID, &allelev1.VerificationRun{
			Id:         row.ID,
			ChangeId:   worktreeID,
			Steps:      steps,
			Status:     overallStatus,
			StartedAt:  now,
			FinishedAt: now,
		})
	case allelev1.VerificationStatus_VERIFICATION_STATUS_RUNNING:
		go s.pollBenchRun(s.bgCtx, row.ID, benchRunID, wt.RepositoryID, worktreeID, steps)
	}

	return row, nil
}

// setWorktreeStatusForVerification maps a merge-queue entry's own overall VerificationStatus onto
// the worktree's own WorktreeStatus -- VERIFYING is deliberately used only for RUNNING (a real Bench
// run actively in flight); PASSED/WAITING both still mean "merge not yet executed" (see this plan's
// own found gap: no phase has built real merge execution yet), so both map to QUEUED, same as before
// Phase 10.
func (s *store) setWorktreeStatusForVerification(ctx context.Context, worktreeID string, status allelev1.VerificationStatus) error {
	newStatus := allelev1.WorktreeStatus_WORKTREE_STATUS_QUEUED
	switch status {
	case allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED:
		newStatus = allelev1.WorktreeStatus_WORKTREE_STATUS_BLOCKED
	case allelev1.VerificationStatus_VERIFICATION_STATUS_RUNNING:
		newStatus = allelev1.WorktreeStatus_WORKTREE_STATUS_VERIFYING
	}
	if _, err := s.db.NewUpdate().Model((*worktreeRow)(nil)).Set("status = ?", int32(newStatus)).Where("id = ?", worktreeID).Exec(ctx); err != nil {
		return fmt.Errorf("updating worktree status: %w", err)
	}
	return nil
}

// triggerBenchVerification registers worktreeID's own Bench workflow from verifyYAML (substituting
// Allele's own placeholders first -- see verification.go) and triggers a run. Never returns a Go
// error for a Bench-side failure (a bad verify.yaml, Bench unreachable, ...) -- that's reported as an
// ordinary FAILED VerificationStep instead, the same way a real failing test would be, so a merge
// queue entry's own Checks list is always the single place to look for "why," never split across a
// step list and a separate RPC error.
func (s *store) triggerBenchVerification(ctx context.Context, repo *repositoryRow, wt *worktreeRow, verifyYAML []byte) (step *allelev1.VerificationStep, runID string, err error) {
	const stepName = "Bench verification"

	commitSHA, err := gitutil.HeadCommit(s.repoPath(repo.Name), wt.Branch)
	if err != nil {
		return &allelev1.VerificationStep{Name: stepName, Status: allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED,
			Detail: fmt.Sprintf("resolving HEAD commit: %v", err)}, "", err
	}

	resolved := substitutePlaceholders(verifyYAML, commitSHA, repo.Name, wt.Branch)
	workflowName := "allele-" + wt.ID
	renamed, err := rewriteWorkflowName(resolved, workflowName)
	if err != nil {
		return &allelev1.VerificationStep{Name: stepName, Status: allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED,
			Detail: err.Error()}, "", err
	}

	if err := s.bench.registerWorkflow(ctx, workflowName, renamed); err != nil {
		return &allelev1.VerificationStep{Name: stepName, Status: allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED,
			Detail: err.Error()}, "", err
	}

	runID, err = s.bench.triggerRun(ctx, workflowName)
	if err != nil {
		return &allelev1.VerificationStep{Name: stepName, Status: allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED,
			Detail: err.Error()}, "", err
	}

	return &allelev1.VerificationStep{Name: stepName, Status: allelev1.VerificationStatus_VERIFICATION_STATUS_RUNNING}, runID, nil
}

// pollBenchRun waits for benchRunID to reach a terminal status (or waitForTerminalRun's own
// pollTimeout) and then replaces the merge queue entry's placeholder "Bench verification" step with
// Bench's own real per-step results, updates the worktree's status, and fires VerificationCompleted
// -- the event the synchronous half of EnqueueMerge deliberately skipped for the RUNNING case. Takes
// priorSteps (the AST step plus the RUNNING placeholder) so the final Checks list keeps the AST
// step's own result rather than losing it.
//
// Runs on store.bgCtx, not the originating request's context -- see store.go's own doc comment on
// why a second chassis.Closer() reader would be the wrong way to bound this goroutine's lifetime.
func (s *store) pollBenchRun(ctx context.Context, entryID, benchRunID, repositoryID, worktreeID string, priorSteps []*allelev1.VerificationStep) {
	run, err := s.bench.waitForTerminalRun(ctx, benchRunID)

	finalSteps := make([]*allelev1.VerificationStep, 0, len(priorSteps)+len(run.GetSteps()))
	for _, st := range priorSteps {
		if st.GetName() != "Bench verification" {
			finalSteps = append(finalSteps, st)
		}
	}
	overallStatus := allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED
	if err != nil {
		finalSteps = append(finalSteps, &allelev1.VerificationStep{
			Name: "Bench verification", Status: allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED, Detail: err.Error(),
		})
	} else {
		for _, sr := range run.GetSteps() {
			finalSteps = append(finalSteps, benchStepResultToVerificationStep(sr))
		}
		overallStatus = benchRunStatusToAllele(run.GetStatus())
	}

	checksJSON, marshalErr := marshalChecks(finalSteps)
	if marshalErr != nil {
		s.logBackgroundError("marshaling bench verification result", marshalErr)
		return
	}
	if _, dbErr := s.db.NewUpdate().Model((*mergeQueueEntryRow)(nil)).
		Set("status = ?", int32(overallStatus)).
		Set("checks = ?", checksJSON).
		Where("id = ?", entryID).Exec(ctx); dbErr != nil {
		s.logBackgroundError("updating merge queue entry after bench run", dbErr)
		return
	}
	if dbErr := s.setWorktreeStatusForVerification(ctx, worktreeID, overallStatus); dbErr != nil {
		s.logBackgroundError("updating worktree status after bench run", dbErr)
		return
	}

	now := timestamppb.Now()
	s.events.VerificationCompleted(repositoryID, &allelev1.VerificationRun{
		Id:         entryID,
		ChangeId:   worktreeID,
		BenchRunId: benchRunID,
		Steps:      finalSteps,
		Status:     overallStatus,
		StartedAt:  run.GetStartedAt(),
		FinishedAt: now,
	})
}

// recoverPendingBenchRuns is called once at startup (main.go): a merge queue entry left
// Status=RUNNING with a BenchRunID means its own pollBenchRun goroutine died with whatever process
// enqueued it -- this dev stack's watch-mode restarts make that a routine occurrence, not a rare
// crash, so every such entry gets a fresh poller rather than sitting stuck mid-verification forever.
func (s *store) recoverPendingBenchRuns(ctx context.Context) error {
	if s.bench == nil {
		return nil
	}
	var rows []*mergeQueueEntryRow
	if err := s.db.NewSelect().Model(&rows).
		Where("status = ? AND bench_run_id != ''", int32(allelev1.VerificationStatus_VERIFICATION_STATUS_RUNNING)).
		Scan(ctx); err != nil {
		return fmt.Errorf("listing in-flight bench verifications: %w", err)
	}
	for _, row := range rows {
		steps, err := unmarshalChecks(row.Checks)
		if err != nil {
			s.logBackgroundError("unmarshaling checks for recovered bench run", err)
			continue
		}
		go s.pollBenchRun(s.bgCtx, row.ID, row.BenchRunID, row.RepositoryID, row.ChangeID, steps)
	}
	return nil
}

// logBackgroundError is pollBenchRun/recoverPendingBenchRuns's own error sink -- there is no request
// to return an error to by the time these run (a background goroutine, or startup recovery), so each
// failure is logged and the affected entry is simply left as-is rather than silently lost; a future
// ListMergeQueue/GetChange call still surfaces whatever status it was last known to hold.
func (s *store) logBackgroundError(context string, err error) {
	// A package-level logger isn't threaded through store today (every other method returns errors
	// to an RPC caller instead) -- stderr is the honest, simplest sink for a goroutine with no
	// caller left to report to, consistent with how the pre/post-receive hooks themselves report
	// failures when nothing else is listening.
	fmt.Fprintf(os.Stderr, "allele: background error (%s): %v\n", context, err)
}

func (s *store) ListMergeQueue(ctx context.Context, repositoryID string) ([]*mergeQueueEntryRow, error) {
	var rows []*mergeQueueEntryRow
	q := s.db.NewSelect().Model(&rows)
	if repositoryID != "" {
		q = q.Where("repository_id = ?", repositoryID)
	}
	if err := q.OrderExpr("position ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("listing merge queue: %w", err)
	}
	return rows, nil
}

func (s *store) setMergeQueueHold(ctx context.Context, id string, held bool) (*mergeQueueEntryRow, error) {
	res, err := s.db.NewUpdate().Model((*mergeQueueEntryRow)(nil)).Set("held = ?", held).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating merge queue entry %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("merge queue entry %q: %w", id, sql.ErrNoRows)
	}
	row := new(mergeQueueEntryRow)
	if err := s.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return row, nil
}
