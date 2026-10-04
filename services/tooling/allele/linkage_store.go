package main

import (
	"context"
	"fmt"
	"time"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	"github.com/steady-bytes/draft/services/tooling/allele/gitutil"
	"github.com/steady-bytes/draft/services/tooling/allele/linkage"
)

// GetLinkageManifest reads and parses linkage.ManifestPath from ref -- the repository's own default
// branch when ref is empty, per GetLinkageManifestRequest's own documented default.
func (s *store) GetLinkageManifest(ctx context.Context, repositoryID, ref string) (*allelev1.LinkageManifest, error) {
	repo, err := s.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	if ref == "" {
		ref = repo.DefaultBranch
	}
	path := s.repoPath(repo.Name)

	commitSHA, err := gitutil.HeadCommit(path, ref)
	if err != nil {
		return nil, fmt.Errorf("resolving HEAD for %s: %w", ref, err)
	}
	components, err := s.loadLinkageManifest(path, ref)
	if err != nil {
		return nil, err
	}
	return &allelev1.LinkageManifest{
		RepositoryId: repositoryID,
		CommitSha:    commitSHA,
		Components:   components,
		Policy:       allelev1.BreakingChangePolicy(repo.BreakingChangePolicy),
	}, nil
}

// loadLinkageManifest reads linkage.ManifestPath at ref and parses it -- (nil, nil) (an empty
// component list, not an error) for a repository that has never committed one, matching
// gitutil.ShowFile's own "doesn't exist" convention and EnqueueMerge's identical treatment of a
// missing .allele/verify.yaml (Phase 10): a repository not using this feature yet is a normal,
// expected state, not a failure.
func (s *store) loadLinkageManifest(repoPath, ref string) ([]*allelev1.Component, error) {
	src, err := gitutil.ShowFile(repoPath, ref, linkage.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s from %s: %w", linkage.ManifestPath, ref, err)
	}
	if src == nil {
		return nil, nil
	}
	return linkage.Parse(src)
}

func (s *store) DiffLinkageManifest(ctx context.Context, repositoryID, baseRef, compareRef string) ([]*allelev1.LinkageManifestChange, error) {
	repo, err := s.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	path := s.repoPath(repo.Name)

	base, err := s.loadLinkageManifest(path, baseRef)
	if err != nil {
		return nil, err
	}
	compare, err := s.loadLinkageManifest(path, compareRef)
	if err != nil {
		return nil, err
	}
	return linkage.Diff(base, compare), nil
}

func (s *store) SetBreakingChangePolicy(ctx context.Context, repositoryID string, policy allelev1.BreakingChangePolicy) (*allelev1.LinkageManifest, error) {
	if _, err := s.db.NewUpdate().Model((*repositoryRow)(nil)).
		Set("breaking_change_policy = ?", int32(policy)).
		Where("id = ?", repositoryID).Exec(ctx); err != nil {
		return nil, fmt.Errorf("updating breaking change policy: %w", err)
	}
	return s.GetLinkageManifest(ctx, repositoryID, "")
}

// ApproveLinkageChange persists that a human has signed off on changeID's (a worktree id) own
// breaking linkage changes -- see linkage_model.go's own doc comment on why this isn't yet consulted
// by EnqueueMerge. Upserted, not inserted-once: re-approving (e.g. after the worktree pushed again)
// is a normal action, not an error.
func (s *store) ApproveLinkageChange(ctx context.Context, repositoryID, changeID string) error {
	row := &linkageApprovalRow{RepositoryID: repositoryID, ChangeID: changeID, ApprovedAt: time.Now().UTC()}
	_, err := s.db.NewInsert().Model(row).
		On("CONFLICT (repository_id, change_id) DO UPDATE").
		Set("approved_at = EXCLUDED.approved_at").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("recording linkage approval: %w", err)
	}
	return nil
}
