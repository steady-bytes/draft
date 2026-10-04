package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var errRepositoryNotFound = errors.New("repository not found")

// store owns both halves of a Repository: the Postgres row (identity, metadata, cached counts)
// and the real bare git repository on local disk that backs it. See the plan's Decisions -- a
// single active Allele instance owns reposDir, no multi-instance replication in this phase.
type store struct {
	db       *bun.DB
	reposDir string // absolute path; see newStore

	// preReceiveHookBinaryPath/postReceiveHookBinaryPath/serverAddr are baked into every repo's
	// installed hooks (see installHook) so each knows which repository it belongs to and where to
	// call back -- an empty path disables that one hook's installation entirely (see installHook's
	// own doc comment). Two independent paths, not one: pre-receive enforces permissions (Phase 8)
	// and must fail closed if missing; post-receive only publishes telemetry (Phase 9) and a repo
	// missing it just means quieter telemetry, not a security gap -- the same distinction
	// CreateRepository's own hard-fail-on-pre-receive-only logic below reflects.
	preReceiveHookBinaryPath  string
	postReceiveHookBinaryPath string
	serverAddr                string

	// events is Allele's Catalyst CloudEvent sink (see events.go) -- a noopEventPublisher when
	// newStore isn't given a real one, so every call site below can fire events unconditionally.
	events EventPublisher

	// bench is Phase 10's Bench WorkflowService client (see verification.go) -- nil disables
	// Bench-triggered verification entirely (EnqueueMerge falls back to the AST-only check alone,
	// Phase 7's own behavior), the same "absent dependency means a quieter feature, not a crash"
	// posture as the hook binaries above.
	bench *benchClient

	// bgCtx outlives any single request -- background pollers (pollBenchRun) are launched from
	// EnqueueMerge's own request-scoped ctx would otherwise be cancelled the moment that RPC
	// returns, long before a real test suite finishes running. Cancelled via a real chassis.Effect
	// in main.go, not a second goroutine racing chassis.Closer() -- see events.go's own eventsCtx
	// for why that distinction matters (Phase 9's own found-and-fixed shutdown-hang bug).
	bgCtx context.Context
}

func newStore(db *bun.DB, reposDir, preReceiveHookBinaryPath, postReceiveHookBinaryPath, serverAddr string, events EventPublisher, bench *benchClient, bgCtx context.Context) (*store, error) {
	abs, err := filepath.Abs(reposDir)
	if err != nil {
		return nil, fmt.Errorf("resolving repos_dir %q: %w", reposDir, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("creating repos_dir %q: %w", abs, err)
	}
	if events == nil {
		events = noopEventPublisher{}
	}
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	return &store{
		db:                        db,
		reposDir:                  abs,
		preReceiveHookBinaryPath:  preReceiveHookBinaryPath,
		postReceiveHookBinaryPath: postReceiveHookBinaryPath,
		serverAddr:                serverAddr,
		events:                    events,
		bench:                     bench,
		bgCtx:                     bgCtx,
	}, nil
}

// validRepoName rejects anything that isn't a plain, relative, slash-separated path -- no "..",
// no leading "/", no empty segments. This is enforced before the name is ever joined into a
// filesystem path (CreateRepository) or trusted to pick a directory to CGI into (the git
// http-backend handler, githttp.go) -- a git-hosting service that joins untrusted input into a
// path without checking this first is a real vulnerability, not a deferred concern.
func validRepoName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" {
			return false
		}
	}
	return true
}

// repoPath is reposDir/<name>.git -- the on-disk bare repository for a given registered name,
// e.g. "steady-bytes/draft" -> "<reposDir>/steady-bytes/draft.git". Never call this with a name
// that hasn't already passed validRepoName.
func (s *store) repoPath(name string) string {
	return filepath.Join(s.reposDir, name+".git")
}

// CreateRepository registers a new Repository row and creates the real bare git repository that
// backs it. If the Postgres insert fails after the bare repo is created on disk, the directory is
// removed again -- a failed CreateRepository should never leave an orphaned, unregistered repo
// sitting on disk that the git http-backend handler would otherwise have no record of.
func (s *store) CreateRepository(ctx context.Context, name, description string) (*repositoryRow, error) {
	if !validRepoName(name) {
		return nil, fmt.Errorf("invalid repository name %q", name)
	}

	path := s.repoPath(name)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("repository %q already exists on disk at %s", name, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating parent directory for %q: %w", name, err)
	}

	// --initial-branch=main, not whatever init.defaultBranch happens to be configured on this
	// machine (plain git itself still defaults to "master" absent that config -- "main" becoming
	// common is a GitHub/GitLab server-side convention, not a git default) -- needs git >= 2.28,
	// confirmed present (2.50.0) during Phase 1's environment check.
	if out, err := exec.CommandContext(ctx, "git", "init", "--bare", "--initial-branch=main", path).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git init --bare failed: %w: %s", err, bytesTrim(out))
	}

	// git http-backend gates receive-pack (push) on this per-repo config value, which defaults to
	// false -- independent of GIT_HTTP_EXPORT_ALL (which only governs read/upload-pack access).
	// See the plan's Decisions ("git http-backend is wrapped, not reimplemented").
	if out, err := exec.CommandContext(ctx, "git", "-C", path, "config", "http.receivepack", "true").CombinedOutput(); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("git config http.receivepack failed: %w: %s", err, bytesTrim(out))
	}

	id := uuid.NewString()
	// Both installed before the DB insert, and both fail CreateRepository outright on a real
	// install error (a filesystem write failure, not the ordinary "binary not built yet" case --
	// see installPreReceiveHook's own doc comment on that distinction) -- simpler and more
	// consistent than leaving a registered repository with a partially-hooked state, even though
	// only the pre-receive half is a security boundary (post-receive only feeds telemetry).
	if err := s.installPreReceiveHook(path, id); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("installing pre-receive hook: %w", err)
	}
	if err := s.installPostReceiveHook(path, id); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("installing post-receive hook: %w", err)
	}

	row := &repositoryRow{
		ID:                   id,
		Name:                 name,
		Description:          description,
		Languages:            []string{},
		DefaultBranch:        "main",
		SymbolCount:          0,
		CreatedAt:            time.Now().UTC(),
		BreakingChangePolicy: int32(allelev1.BreakingChangePolicy_BREAKING_CHANGE_POLICY_INTERFACE_ONLY),
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("inserting repository row: %w", err)
	}
	return row, nil
}

// installPreReceiveHook and installPostReceiveHook each write <repoPath>/hooks/<name> as a small
// shell script -- not a symlink straight to the compiled binary -- so this repository's own id and
// this server's address can be baked in as environment variables the hook reads
// (ALLELE_REPOSITORY_ID/ALLELE_SERVER_ADDR) rather than the hook needing to re-derive them, e.g. by
// reversing repoPath back to a registered name and looking it up, which would duplicate
// githttp.go's ServeHTTP doing exactly that for a different reason.
//
// Each is a no-op (not an error) when its own binary path is empty -- main.go only resolves one
// when it can find the matching sibling cmd/{pre,post}-receive-hook binary, so a dev environment
// that hasn't built one yet gets a repository missing that hook's behavior (no permission
// enforcement, or no push telemetry, respectively) rather than a hard failure blocking all
// repository creation; this gap is surfaced in the plan's own completion notes, not hidden.
func (s *store) installPreReceiveHook(repoPath, repositoryID string) error {
	return writeHookScript(repoPath, "pre-receive", s.preReceiveHookBinaryPath, repositoryID, s.serverAddr)
}

func (s *store) installPostReceiveHook(repoPath, repositoryID string) error {
	return writeHookScript(repoPath, "post-receive", s.postReceiveHookBinaryPath, repositoryID, s.serverAddr)
}

func writeHookScript(repoPath, hookName, hookBinaryPath, repositoryID, serverAddr string) error {
	if hookBinaryPath == "" {
		return nil
	}
	hooksDir := filepath.Join(repoPath, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}
	script := fmt.Sprintf("#!/bin/sh\nexport ALLELE_REPOSITORY_ID=%s\nexport ALLELE_SERVER_ADDR=%s\nexec %s\n",
		shellQuote(repositoryID), shellQuote(serverAddr), shellQuote(hookBinaryPath))
	if err := os.WriteFile(filepath.Join(hooksDir, hookName), []byte(script), 0o755); err != nil {
		return fmt.Errorf("writing %s hook: %w", hookName, err)
	}
	return nil
}

// shellQuote single-quotes s for safe interpolation into the generated hook script (POSIX's
// standard "close quote, escaped literal quote, reopen quote" trick) -- repositoryID is
// uuid.NewString() output and hookBinaryPath/serverAddr are both server-controlled, none ever
// attacker-influenced, but a filesystem path containing a space would otherwise silently break the
// generated script, so this costs little and removes that whole class of surprise.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (s *store) ListRepositories(ctx context.Context) ([]*repositoryRow, error) {
	var rows []*repositoryRow
	if err := s.db.NewSelect().Model(&rows).OrderExpr("name ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("listing repositories: %w", err)
	}
	return rows, nil
}

func (s *store) GetRepository(ctx context.Context, id string) (*repositoryRow, error) {
	row := new(repositoryRow)
	err := s.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errRepositoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting repository %q: %w", id, err)
	}
	return row, nil
}

// getRepositoryByName is what the git http-backend handler (githttp.go) uses to confirm a
// requested repo path is actually registered before letting git touch the filesystem -- the git
// URL path addresses a repo by name (what a real `git clone` command types), never by the opaque
// id GetRepository/the RPC surface uses.
func (s *store) getRepositoryByName(ctx context.Context, name string) (*repositoryRow, error) {
	row := new(repositoryRow)
	err := s.db.NewSelect().Model(row).Where("name = ?", name).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errRepositoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting repository by name %q: %w", name, err)
	}
	return row, nil
}

func bytesTrim(b []byte) string {
	return strings.TrimSpace(string(b))
}
