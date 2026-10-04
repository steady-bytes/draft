package main

import (
	"fmt"
	"net/http"
	"net/http/cgi"
	"os/exec"
	"regexp"
	"strings"

	"github.com/steady-bytes/draft/pkg/chassis"
)

// gitPathPattern matches the three smart-HTTP paths this service supports: GET /{repo}/info/refs
// (ref advertisement for both fetch and push) and POST /{repo}/git-upload-pack (fetch/clone) and
// POST /{repo}/git-receive-pack (push). {repo} itself may contain slashes (e.g.
// "steady-bytes/draft"), hence the non-greedy capture up to the last matching suffix rather than a
// single path segment. Dumb-HTTP fallback paths (/HEAD, /objects/...) are deliberately
// unsupported -- see the plan's Decisions: any git client built since ~2010 always tries smart
// HTTP first via ?service=... on /info/refs, so dumb-protocol support buys nothing a modern client
// needs.
var gitPathPattern = regexp.MustCompile(`^/(.+?)/(info/refs|git-upload-pack|git-receive-pack)$`)

// repoNameFromPath extracts {repo} from a request path matching gitPathPattern, with a trailing
// ".git" stripped if present (git clients commonly send both "name" and "name.git" remotes
// interchangeably; Allele's own registry stores names without the suffix).
func repoNameFromPath(path string) (name string, ok bool) {
	m := gitPathPattern.FindStringSubmatch(path)
	if m == nil {
		return "", false
	}
	return strings.TrimSuffix(m[1], ".git"), true
}

// gitHandler wraps `git http-backend` behind net/http/cgi -- see the plan's Decisions ("git
// http-backend is wrapped, not reimplemented or replaced by a pure-Go git stack"), the same
// approach Gitea and Gogs both took for their own smart-HTTP serving. It is registered on
// chassis's shared mux at "/" (see RegisterRPC below), the catch-all fallback behind
// AlleleService's own, more specific Connect path -- http.ServeMux resolves overlapping patterns
// by longest-prefix-wins, so this never shadows the RPC handler registered alongside it.
//
// Before delegating to the real git http-backend subprocess, ServeHTTP confirms the requested repo
// is actually registered (store.getRepositoryByName) -- a 404 here is a clean, intentional
// rejection, not git http-backend's own (equally correct, but less informative) response to a
// missing directory. This is also where Phase 8's permission enforcement will plug in later.
type gitHandler struct {
	store *store
	cgi   http.Handler
}

func newGitHandler(st *store) (*gitHandler, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git not found on PATH: %w", err)
	}
	return &gitHandler{
		store: st,
		cgi: &cgi.Handler{
			Path: gitPath,
			Args: []string{"http-backend"},
			Env: []string{
				"GIT_PROJECT_ROOT=" + st.reposDir,
				// Export every repo under reposDir for read access (upload-pack/clone/fetch).
				// Push access is a separate, per-repo config value (http.receivepack) set at
				// CreateRepository time -- see store.go.
				"GIT_HTTP_EXPORT_ALL=1",
			},
		},
	}, nil
}

func (h *gitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name, ok := repoNameFromPath(r.URL.Path)
	if !ok || !validRepoName(name) {
		http.NotFound(w, r)
		return
	}
	if _, err := h.store.getRepositoryByName(r.Context(), name); err != nil {
		http.NotFound(w, r)
		return
	}
	h.cgi.ServeHTTP(w, r)
}

// RegisterRPC mounts the git handler onto chassis's shared mux at "/" -- see the type comment for
// why AddHandler (not a second listener) is the right primitive here, matching exactly how
// Bench's webhookHandler (bench/webhook.go) mounts its own non-Connect HTTP receiver alongside its
// Connect service on the same mux.
func (h *gitHandler) RegisterRPC(server chassis.Rpcer) {
	server.AddHandler("/", h, false)
}
