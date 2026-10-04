package main

import (
	"context"
	"errors"
	"strconv"
	"strings"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	"github.com/steady-bytes/draft/api/tooling/allele/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

// handler implements v1connect.AlleleServiceHandler. Repositories (List/Create/Get) are real as
// of Phase 3; every other method is CodeUnimplemented until its own phase lands -- the same
// precise-scope-matching Relay's own Phase 4/5 handlers used (e.g. DiscardRecording/RenameSpeaker
// left unimplemented until the phase that actually owns them), rather than a handler that looks
// complete but silently does nothing.
type handler struct {
	store *store
}

func newHandler(st *store) *handler {
	return &handler{store: st}
}

// RegisterRPC mounts AlleleService onto chassis's shared mux. See githttp.go's gitHandler for the
// other, non-Connect registrant sharing this same mux.
func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, connectHandler := v1connect.NewAlleleServiceHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, connectHandler, true)
}

func unimplemented(phase string) error {
	return connect.NewError(connect.CodeUnimplemented, errors.New("not implemented until "+phase))
}

// -- Repositories -------------------------------------------------------------------------------

func (h *handler) ListRepositories(ctx context.Context, req *connect.Request[allelev1.ListRepositoriesRequest]) (*connect.Response[allelev1.ListRepositoriesResponse], error) {
	rows, err := h.store.ListRepositories(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	repos := make([]*allelev1.Repository, 0, len(rows))
	for _, r := range rows {
		repos = append(repos, r.toProto())
	}
	return connect.NewResponse(&allelev1.ListRepositoriesResponse{Repositories: repos}), nil
}

func (h *handler) CreateRepository(ctx context.Context, req *connect.Request[allelev1.CreateRepositoryRequest]) (*connect.Response[allelev1.Repository], error) {
	name := req.Msg.GetName()
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	row, err := h.store.CreateRepository(ctx, name, req.Msg.GetDescription())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(row.toProto()), nil
}

func (h *handler) GetRepository(ctx context.Context, req *connect.Request[allelev1.GetRepositoryRequest]) (*connect.Response[allelev1.Repository], error) {
	row, err := h.store.GetRepository(ctx, req.Msg.GetId())
	if errors.Is(err, errRepositoryNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(row.toProto()), nil
}

// -- Worktrees (Phase 7) ------------------------------------------------------------------------

func (h *handler) ListWorktrees(ctx context.Context, req *connect.Request[allelev1.ListWorktreesRequest]) (*connect.Response[allelev1.ListWorktreesResponse], error) {
	rows, err := h.store.ListWorktrees(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	worktrees := make([]*allelev1.Worktree, 0, len(rows))
	for _, r := range rows {
		worktrees = append(worktrees, r.toProto())
	}
	return connect.NewResponse(&allelev1.ListWorktreesResponse{Worktrees: worktrees}), nil
}

func (h *handler) GetWorktreeOverlap(ctx context.Context, req *connect.Request[allelev1.GetWorktreeOverlapRequest]) (*connect.Response[allelev1.GetWorktreeOverlapResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "allele.get_worktree_overlap")
	span.SetBusinessAttribute("repository_id", req.Msg.GetRepositoryId())

	pairs, err := h.store.GetOverlapMatrix(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		span.End(err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	conflictCount := 0
	for _, p := range pairs {
		if p.GetKind() == allelev1.OverlapKind_OVERLAP_KIND_SEMANTIC_CONFLICT {
			conflictCount++
		}
	}
	span.SetRuntimeAttribute("pair_count", strconv.Itoa(len(pairs)))
	span.SetRuntimeAttribute("conflict_count", strconv.Itoa(conflictCount))
	span.End(nil)

	return connect.NewResponse(&allelev1.GetWorktreeOverlapResponse{Pairs: pairs}), nil
}

func (h *handler) CloseWorktree(ctx context.Context, req *connect.Request[allelev1.CloseWorktreeRequest]) (*connect.Response[allelev1.CloseWorktreeResponse], error) {
	if err := h.store.CloseWorktree(ctx, req.Msg.GetWorktreeId()); err != nil {
		if errors.Is(err, errWorktreeNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&allelev1.CloseWorktreeResponse{}), nil
}

// -- Changes (Phases 5-7) -------------------------------------------------------------------------
//
// A Change's id is always a worktree id -- Change is computed fresh from git on every call, never
// persisted, so it can never go stale relative to a new push the way a cached row could (see
// store.go's computeChange). ListChanges/GetChange both resolve through the same worktree lookup.

func (h *handler) ListChanges(ctx context.Context, req *connect.Request[allelev1.ListChangesRequest]) (*connect.Response[allelev1.ListChangesResponse], error) {
	worktrees, err := h.store.ListWorktrees(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	changes := make([]*allelev1.Change, 0, len(worktrees))
	for _, wt := range worktrees {
		if wt.Status == int32(allelev1.WorktreeStatus_WORKTREE_STATUS_MERGED) {
			continue
		}
		change, err := h.store.computeChange(ctx, wt)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		changes = append(changes, change)
	}
	return connect.NewResponse(&allelev1.ListChangesResponse{Changes: changes}), nil
}

func (h *handler) GetChange(ctx context.Context, req *connect.Request[allelev1.GetChangeRequest]) (*connect.Response[allelev1.Change], error) {
	wt, err := h.store.GetWorktree(ctx, req.Msg.GetId())
	if errors.Is(err, errWorktreeNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	change, err := h.store.computeChange(ctx, wt)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(change), nil
}

func (h *handler) RebaseChange(ctx context.Context, req *connect.Request[allelev1.RebaseChangeRequest]) (*connect.Response[allelev1.Change], error) {
	return nil, unimplemented("Phase 6 (Three-way merge + conflict detection) -- RebaseChange needs to actually rewrite history, not just classify it; reconciliation itself (merge.Overlap) is built and live in GetChange/GetWorktreeOverlap above")
}

func (h *handler) EnqueueMerge(ctx context.Context, req *connect.Request[allelev1.EnqueueMergeRequest]) (*connect.Response[allelev1.MergeQueueEntry], error) {
	ctx, span := chassis.StartSpan(ctx, "allele.enqueue_merge")
	span.SetBusinessAttribute("change_id", req.Msg.GetChangeId())
	// worktree_id is the same value as change_id (Change.Id and Worktree.ID are the same thing --
	// see worktree_store.go's computeChange), named separately here only because Telemetry's own
	// table names both attributes explicitly.
	span.SetBusinessAttribute("worktree_id", req.Msg.GetChangeId())

	row, err := h.store.EnqueueMerge(ctx, req.Msg.GetChangeId())
	if errors.Is(err, errWorktreeNotFound) {
		span.End(err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		span.End(err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	span.SetBusinessAttribute("repository_id", row.RepositoryID)
	span.End(nil)

	entry, err := row.toProto()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(entry), nil
}

// -- Merge queue (Phase 7) ------------------------------------------------------------------------

func (h *handler) ListMergeQueue(ctx context.Context, req *connect.Request[allelev1.ListMergeQueueRequest]) (*connect.Response[allelev1.ListMergeQueueResponse], error) {
	rows, err := h.store.ListMergeQueue(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	entries := make([]*allelev1.MergeQueueEntry, 0, len(rows))
	for _, r := range rows {
		entry, err := r.toProto()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		entries = append(entries, entry)
	}
	return connect.NewResponse(&allelev1.ListMergeQueueResponse{Entries: entries}), nil
}

func (h *handler) HoldMergeQueueEntry(ctx context.Context, req *connect.Request[allelev1.HoldMergeQueueEntryRequest]) (*connect.Response[allelev1.MergeQueueEntry], error) {
	row, err := h.store.setMergeQueueHold(ctx, req.Msg.GetEntryId(), true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	entry, err := row.toProto()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(entry), nil
}

func (h *handler) ResumeMergeQueueEntry(ctx context.Context, req *connect.Request[allelev1.ResumeMergeQueueEntryRequest]) (*connect.Response[allelev1.MergeQueueEntry], error) {
	row, err := h.store.setMergeQueueHold(ctx, req.Msg.GetEntryId(), false)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	entry, err := row.toProto()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(entry), nil
}

// -- Live feed (Phase 12) -------------------------------------------------------------------------
//
// Relabeled from an earlier "Phase 9" placeholder: Phase 9's own checklist scope is Catalyst events/
// Foundry/telemetry, and WatchChanges is explicitly a *separate*, direct in-process stream, not a
// Catalyst Consume subscription (see its own doc comment in the proto) -- it has no real consumer
// to verify it against until Phase 12's web client exists to open one, so it stays deferred to then
// rather than shipped unverified now.

func (h *handler) WatchChanges(ctx context.Context, req *connect.Request[allelev1.WatchChangesRequest], stream *connect.ServerStream[allelev1.SymbolChangeEvent]) error {
	return unimplemented("Phase 12 (Web client) -- a direct in-process stream with no real consumer to verify it against until then")
}

// -- Push notifications (Phase 9) ------------------------------------------------------------------

// NotifyPush is called by the post-receive hook, once per accepted ref update. Not called by UI/API
// clients.
func (h *handler) NotifyPush(ctx context.Context, req *connect.Request[allelev1.NotifyPushRequest]) (*connect.Response[allelev1.NotifyPushResponse], error) {
	if err := h.store.NotifyPush(ctx, req.Msg.GetRepositoryId(), req.Msg.GetRef()); err != nil {
		if errors.Is(err, errRepositoryNotFound) || errors.Is(err, errWorktreeNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&allelev1.NotifyPushResponse{}), nil
}

// -- Provenance (Phase 8) -------------------------------------------------------------------------

func (h *handler) GetProvenance(ctx context.Context, req *connect.Request[allelev1.GetProvenanceRequest]) (*connect.Response[allelev1.Provenance], error) {
	changeID := req.Msg.GetChangeId()
	row, err := h.store.GetProvenanceByChangeID(ctx, changeID)
	if errors.Is(err, errWorktreeNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, errProvenanceNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	proto, err := row.toProto(changeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(proto), nil
}

func (h *handler) ListPathPermissions(ctx context.Context, req *connect.Request[allelev1.ListPathPermissionsRequest]) (*connect.Response[allelev1.ListPathPermissionsResponse], error) {
	rows, err := h.store.ListPathPermissions(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	perms := make([]*allelev1.PathPermission, 0, len(rows))
	for _, r := range rows {
		perms = append(perms, r.toProto())
	}
	return connect.NewResponse(&allelev1.ListPathPermissionsResponse{Permissions: perms}), nil
}

func (h *handler) SetPathPermissions(ctx context.Context, req *connect.Request[allelev1.SetPathPermissionsRequest]) (*connect.Response[allelev1.SetPathPermissionsResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "allele.set_path_permissions")
	repositoryID := req.Msg.GetRepositoryId()
	span.SetBusinessAttribute("repository_id", repositoryID)
	// identity_pattern: this RPC replaces a repository's *whole* permission set in one call (see
	// store.go's SetPathPermissions), so there isn't always exactly one identity to name the way
	// Telemetry's own singular attribute name suggests -- every pattern in the request, joined, is
	// the honest value rather than picking just the first.
	patterns := make([]string, 0, len(req.Msg.GetPermissions()))
	for _, p := range req.Msg.GetPermissions() {
		patterns = append(patterns, p.GetIdentityPattern())
	}
	span.SetBusinessAttribute("identity_pattern", strings.Join(patterns, ","))

	if repositoryID == "" {
		err := errors.New("repository_id is required")
		span.End(err)
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	in := make([]*pathPermissionRow, 0, len(req.Msg.GetPermissions()))
	for _, p := range req.Msg.GetPermissions() {
		in = append(in, &pathPermissionRow{
			IdentityPattern: p.GetIdentityPattern(),
			AllowGlobs:      p.GetAllowGlobs(),
			DenyGlobs:       p.GetDenyGlobs(),
		})
	}
	rows, err := h.store.SetPathPermissions(ctx, repositoryID, in)
	if err != nil {
		span.End(err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	span.End(nil)

	perms := make([]*allelev1.PathPermission, 0, len(rows))
	for _, r := range rows {
		perms = append(perms, r.toProto())
	}
	return connect.NewResponse(&allelev1.SetPathPermissionsResponse{Permissions: perms}), nil
}

// CheckPushPermission is called by the pre-receive hook, not UI/API clients -- see the proto's own
// doc comment. An allowed push also records a Provenance row for new_commit_sha (R4.1): recording
// lives here, as a side effect of the allow decision, rather than as a call the hook makes
// separately, since the hook only ever needs "check and record" done together as one round trip.
func (h *handler) CheckPushPermission(ctx context.Context, req *connect.Request[allelev1.CheckPushPermissionRequest]) (*connect.Response[allelev1.CheckPushPermissionResponse], error) {
	repositoryID := req.Msg.GetRepositoryId()
	identity := req.Msg.GetIdentity()
	paths := req.Msg.GetPaths()

	allowed, reason, err := h.store.CheckPushPermission(ctx, repositoryID, identity, paths)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if allowed {
		ownerKind := h.store.ownerKindForRef(ctx, repositoryID, req.Msg.GetRef())
		if err := h.store.RecordProvenance(ctx, req.Msg.GetNewCommitSha(), repositoryID, identity, ownerKind, paths); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	return connect.NewResponse(&allelev1.CheckPushPermissionResponse{Allowed: allowed, Reason: reason}), nil
}

// -- Signing keys (Phase 8) -----------------------------------------------------------------------

func (h *handler) RegisterSigningKey(ctx context.Context, req *connect.Request[allelev1.RegisterSigningKeyRequest]) (*connect.Response[allelev1.SigningKey], error) {
	return nil, unimplemented("Phase 8 (Provenance and permission enforcement)")
}

func (h *handler) ListSigningKeys(ctx context.Context, req *connect.Request[allelev1.ListSigningKeysRequest]) (*connect.Response[allelev1.ListSigningKeysResponse], error) {
	return nil, unimplemented("Phase 8 (Provenance and permission enforcement)")
}

func (h *handler) RevokeSigningKey(ctx context.Context, req *connect.Request[allelev1.RevokeSigningKeyRequest]) (*connect.Response[allelev1.RevokeSigningKeyResponse], error) {
	return nil, unimplemented("Phase 8 (Provenance and permission enforcement)")
}

// -- Linkage manifest (Phase 11) ------------------------------------------------------------------
//
// EnqueueMerge does not yet consult GetLinkageManifest/DiffLinkageManifest/the stored
// BreakingChangePolicy, or ApproveLinkageChange's own recorded approvals, when deciding a merge
// queue entry's PASSED/FAILED status -- every RPC below is real and independently correct (parses
// and diffs a real draft.linkage.yaml, persists a real per-repository policy, persists a real
// approval record), but "a breaking change blocks the queue until approved" is a disclosed,
// not-yet-wired next increment. See linkage_model.go's own doc comment and this phase's completion
// note.

func (h *handler) GetLinkageManifest(ctx context.Context, req *connect.Request[allelev1.GetLinkageManifestRequest]) (*connect.Response[allelev1.LinkageManifest], error) {
	manifest, err := h.store.GetLinkageManifest(ctx, req.Msg.GetRepositoryId(), req.Msg.GetRef())
	if errors.Is(err, errRepositoryNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(manifest), nil
}

func (h *handler) DiffLinkageManifest(ctx context.Context, req *connect.Request[allelev1.DiffLinkageManifestRequest]) (*connect.Response[allelev1.DiffLinkageManifestResponse], error) {
	changes, err := h.store.DiffLinkageManifest(ctx, req.Msg.GetRepositoryId(), req.Msg.GetBaseRef(), req.Msg.GetCompareRef())
	if errors.Is(err, errRepositoryNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&allelev1.DiffLinkageManifestResponse{Changes: changes}), nil
}

func (h *handler) SetBreakingChangePolicy(ctx context.Context, req *connect.Request[allelev1.SetBreakingChangePolicyRequest]) (*connect.Response[allelev1.LinkageManifest], error) {
	manifest, err := h.store.SetBreakingChangePolicy(ctx, req.Msg.GetRepositoryId(), req.Msg.GetPolicy())
	if errors.Is(err, errRepositoryNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(manifest), nil
}

func (h *handler) ApproveLinkageChange(ctx context.Context, req *connect.Request[allelev1.ApproveLinkageChangeRequest]) (*connect.Response[allelev1.ApproveLinkageChangeResponse], error) {
	if err := h.store.ApproveLinkageChange(ctx, req.Msg.GetRepositoryId(), req.Msg.GetChangeId()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&allelev1.ApproveLinkageChangeResponse{}), nil
}
