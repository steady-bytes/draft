---
weight: 48
title: 'Allele — Agentic Git Server Implementation Plan'
description: 'System design, data model, and RPC interface for Allele, Draft''s AST-aware git server: semantic conflict detection, parallel agent worktrees, verified merges, and provenance enforcement.'
icon: 'account_tree'
draft: false
toc: true
---

{{< alert context="info" text="Phase 1 (scaffolding) complete as of 2026-10-01 — services/tooling/allele starts, registers with Blueprint, and holds a real Postgres connection, live-verified against the running local stack. Everything else (git hosting, the structural diff/merge core, worktrees, provenance, verification, the linkage manifest, the web client) is designed here but not yet built. See the Implementation Plan's per-phase notes." />}}

Allele is the git server described in `docs/website/content/docs/allele-prd.md` and depicted in
`mockups/pages/allele-repos.html`, `allele-change.html`, `allele-worktrees.html`,
`allele-linkage.html`, and `allele-evolution.html` (the last of which is partly out of this plan's
scope — see [Relationship to the Agent Service, the Evolution Plugin, and Auth](#relationship-to-the-agent-service-the-evolution-plugin-and-auth)
below). Its core differentiator is AST-level diffing: conflicts are found and resolved by comparing
what changed in a file's syntax tree, not which lines moved, so many agents can commit to the same
codebase in parallel without a human untangling every merge by hand. The PRD organizes this around
four concepts — parallel agent worktrees (§6.1), semantic conflict detection (§6.2), structured
verification loops (§6.3), and provenance/permission enforcement (§6.4) — plus the linkage manifest
(§7), a versioned, diffable graph of component composition using the same diff/merge machinery as
code.

Allele is in the **tooling** domain (`services/tooling/allele`), alongside Bench, Foundry, Lineman,
and Relay — not a core cluster service in the sense Blueprint/Fuse/Catalyst/Chassis are (per
`CLAUDE.md`'s own four-component definition of "core"). It's a specific capability built on top of
the cluster's core services, the same relationship Relay has to audio.

## Relationship to the Agent Service, the Evolution Plugin, and Auth

**The Agent Service (`agents.md`) is where "an agent" already means something — Allele reuses that
identity, it doesn't mint a second one.** An agent committing to Allele is an Executor from the
existing Agent Service, acting under a specific `objective_id`/`task_id`. Allele's provenance
record (§6.4, R4.1) stores those ids directly, as plain string fields rather than a proto import
across domains — the same arm's-length coupling [Relay uses for Lineman's task
ids](/docs/architecture/relay-implementation-plan#relationship-to-lineman-and-beacon) ("Relay has
no task/board model of its own"). Allele has no task/objective model of its own either. This means
"Task T-101 · Roll out Fuse load balancing," shown in `allele-change.html`'s Provenance panel, is a
real foreign reference into the Agent Service's Orchestrator state, not a free-text label Allele
invented.

**Allele does not implement the genetic-algorithm evolution engine depicted in
`allele-evolution.html`.** The PRD is explicit about this (§1's naming note): Allele and the
evolution plugin are "independent components," and the PRD "does not assume any dependency between
them." What Allele *does* provide, and must provide generically rather than special-cased, is
everything an evolution run needs as a *client*: the ability to open many short-lived branches
against one repository concurrently, per-branch AST diffing/verification against `main`, and change
events detailed enough for an external fitness-scoring system to consume. The Worktrees page's
`ev-2` row is the UI-level evidence of this — it renders in the same table, the same overlap matrix,
and the same merge queue as every agent worktree, distinguished only by a `WorktreeOwnerKind` of
`EVOLUTION_RUN` rather than `AGENT` (see [Data Model](#data-model)). "Promote to review," the
evolution mockup's own hand-off action, is just that separate plugin calling Allele's existing
`EnqueueMerge` RPC — Allele needs no awareness of fitness, generations, or populations to support
it. Building the evolution run detail page itself (fitness-by-generation chart, population table,
lineage tree) is out of scope for this plan; it belongs to that plugin's own future implementation
plan.

**Request-time authentication reuses Draft's existing Authentik M2M pattern
([`authentication.md`](/docs/architecture/authentication)) — Allele does not invent a second
credential system to answer "is this caller allowed to talk to Allele at all."** Every Allele route
(UI and git traffic alike — see [System Design](#system-design)) is registered with Fuse like any
other route and gated by the same `RouteAuth`/`ext_authz`/Authentik outpost flow every other service
uses. What Allele adds on top is git-specific and finer-grained than route-level groups/scopes: real
per-path write permission (R4.2) and a durable, independently-verifiable commit signature (R4.3) —
see [Provenance and Permission Enforcement](#provenance-and-permission-enforcement-r4).

## System Design

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                                  Draft Cluster                                   │
│                                                                                    │
│  ┌───────────┐   ┌──────────┐   ┌───────────┐   ┌────────────┐   ┌─────────────┐ │
│  │ Blueprint │   │   Fuse   │   │ Catalyst  │   │   Bench    │   │Agent Service│ │
│  │ (registry)│   │ (routing,│   │ (events)  │   │(verification│  │(Orchestrator│ │
│  │           │   │  UI host)│   │           │   │ workflows) │   │ identity)   │ │
│  └─────┬─────┘   └────┬─────┘   └─────┬─────┘   └─────┬──────┘   └──────┬──────┘ │
│        │              │               │               │                 │        │
│        │     ┌────────▼───────────────▼───────────────▼─────────────────▼──┐     │
│        └────▶│              Allele — services/tooling/allele                │     │
│               │  - AlleleService RPCs (repos, worktrees, changes, queue,    │     │
│               │    provenance, linkage)                                    │     │
│               │  - plain-HTTP git smart endpoints (info/refs, upload-pack,  │     │
│               │    receive-pack) mounted via chassis.AddHandler, wrapping   │     │
│               │    `git http-backend` — see Decisions                      │     │
│               │  - pre-receive hook → structural index + conflict engine    │     │
│               │    (tree-sitter parse, GumTree-style match, 3-way merge)    │     │
│               │  - Postgres: repos, worktrees, changes, provenance,         │     │
│               │    path permissions, linkage read-model                    │     │
│               │  - local disk: bare git repositories (one active instance) │     │
│               │  - Dioxus web client (allele.draft.localhost)              │     │
│               └──────────────────────┬────────────────────────────────────┘     │
│                                       │ publishes manifest                        │
│                                ┌──────▼──────┐                                    │
│                                │   Foundry    │                                    │
│                                │  (catalog)  │                                    │
│                                └─────────────┘                                    │
└────────────────────────────────────────────────────────────────────────────────────┘
```

- **Blueprint** is where Allele registers, exactly like any other chassis service. Allele's own
  domain data (repos, worktrees, changes, provenance, permissions) lives in **Postgres**, not
  Blueprint's KV store — see [Decisions](#decisions) for why, matching Relay's and Bench's reasoning
  almost exactly: the overlap matrix, path-glob permission checks, and change history are real
  relational queries a raft-replicated KV store answers poorly.
- **Fuse** routes `allele.draft.localhost` for the Dioxus UI and `AlleleService`'s own RPC prefix,
  the same subdomain convention every other service's UI uses. It *also* routes the git
  smart-HTTP paths (`/{repo}/info/refs`, `/{repo}/git-upload-pack`, `/{repo}/git-receive-pack`) as
  plain HTTP — these are mounted on chassis's own mux via `AddHandler`
  (`pkg/chassis/rpc.go`), the same mechanism [Bench's webhook
  receiver](/docs/architecture/bench-workflow-engine#the-webhook-trigger-and-status-api) already
  uses for non-Connect HTTP traffic, not a second listener or a bypass of Fuse/Envoy.
- **Catalyst** carries `allele.symbol.*`, `allele.conflict.detected`, `allele.verification.*`, and
  `allele.linkage.changed` CloudEvents — see [Telemetry](#telemetry).
- **Bench** runs Allele's structured verification loops (R3) as ordinary Bench workflows — Allele
  doesn't build a second DAG/step engine. See [Structured Verification
  Loops](#structured-verification-loops-r3).
- **Agent Service** is the source of agent identity for provenance, as described above.
- **Foundry** gets a published plugin manifest at startup via `chassis.Effect`, discovery only — the
  exact precedent [Lineman](/docs/architecture/lineman-implementation-plan#non-goals), Bench, and
  Relay all already established. Allele is a standalone application with its own UI, not a
  `StepExecutor`.
- **The web client** is Dioxus (`services/tooling/allele/web-client`), following
  Lineman/Blueprint/Relay's pattern — own crate, static `d-rail` nav matching the mockups' fixed IA,
  `draft-ui` components throughout, `go:embed`-ed into the Go binary the same way
  `services/tooling/lineman/main.go` embeds its own `web-client/target/dx/.../public`.

## AST-level structural diff and 3-way merge (the hard part)

**Parsing is a reused dependency; the diff/merge core is new engineering — matching exactly what
the PRD's own §8 says, worth being precise about which parts are which.** Parsing uses
`github.com/tree-sitter/go-tree-sitter` (the tree-sitter project's own Go bindings, cgo-based) with
the official `tree-sitter-go` and `tree-sitter-rust` grammars. `.proto` is the one language in the
PRD's Phase 0 scope (§8.4) without an equally mature, widely-used tree-sitter grammar as of this
writing; the plan is to vet the best available community grammar during [Phase
4](#implementation-plan) and ship structural diffing for it if it holds up, falling back to plain
text diff for `.proto` specifically — a disclosed, scoped trim for one language, not a silent cut —
if nothing adequate is found. The linkage manifest's own file format (see below) uses
`tree-sitter-yaml`, which is mature.

**No off-the-shelf library does GumTree-style AST diffing as an embeddable Go package, and neither
GumTree nor difftastic solve the actual problem Allele needs (3-way merge), only 2-way display
diffing.** The PRD cites both as reference implementations for the *pattern* ("one structural-diff
core, N grammars feeding it," §8.2) — not as vendorable dependencies, and this plan treats them the
same way: studied, not shipped. Concretely:

- **Two-way structural diff** — matching nodes between two trees (a change's branch vs. its merge
  base) via GumTree's published two-phase algorithm (greedy top-down matching of isomorphic
  subtrees, then bottom-up matching of modified-but-similar ones by a similarity threshold) — is
  genuinely similar to what GumTree and difftastic already do, and is reimplemented once in Go
  against `go-tree-sitter`'s generic `Node` API, not per language (PRD §8.2, R2.4).
- **Reconciling two edit scripts into one 3-way merge** — deciding whether two sides' changes to the
  *same* node are disjoint (auto-merge) or genuinely clash (flag) — is the part neither reference
  tool provides, and is where R2.2's real subtlety lives. The mockup's own worked example (two
  agents renaming the same parameter for unrelated reasons) only auto-merges correctly if Allele
  knows a function-parameter node has independently-editable name and type children; a node-kind's
  move/rename/retype some fields are independent, and some aren't. This means the "generic diff
  core" promise (NFR Extensibility: "adding a new language should mean integrating a tree-sitter
  grammar... not extending the core diff algorithm") holds for the *matching* algorithm but not
  completely for *merge reconciliation*: each language needs a small, bounded table of "which
  sub-fields of which node kinds are independently mergeable," maintained once per grammar
  integration, not per feature. This is new, Allele-specific engineering — flagged explicitly here
  so it isn't under-scoped later.
- **Surfacing *why*** (R2.3) falls out of the same reconciliation step: a `ConflictReport` records
  which node, which other worktree, and which specific field clashed (or which reference broke, the
  R2.2 case — `route.Upstream.Address` removed in `wt-04`, still read by `wt-07`) rather than a line
  range. See [Data Model](#data-model)'s `ConflictReport`.

## Agent worktrees are branches, and verification is queue-gated, not push-gated

**A "worktree" in the UI is an isolated branch plus Allele's own continuously-updated AST index for
it — never a literal, persistently checked-out working directory on Allele's own disk.** Git already
gives per-ref concurrency for free: pushes to different branches never contend with each other at
the protocol level, so R1.1 ("creating a worktree must not block or be blocked by any other
worktree's operations") and R1.2 (isolation until merged) are close to free, inherited from git
itself, not new concurrency engineering Allele has to build. This also means R1.3's concurrency
target — still an open question, see below — mostly bounds *disk and indexing* capacity, not a
locking scheme. The ephemeral `git worktree add --detach` checkout git itself supports is used only
transiently, by [verification](#structured-verification-loops-r3), and torn down after — the one
place an actual checked-out tree is genuinely needed.

This splits into two already-answered problems rather than one open "where does the sandbox run"
question. The AST merge-simulation step only parses and diffs the pushed code — it never executes
it — so it's safe to run in-process on Allele itself with no isolation beyond the checkout. Actually
*executing* untrusted build/test commands is entirely Bench's job (already how Bench dispatches
`grpc-call`/`foundry://` steps today); wherever Bench already isolates that execution is the answer,
not a new decision this plan introduces.

**Pushing to your own branch is always accepted immediately; AST conflict-checking and verification
happen when a change enters the merge queue, not at push time.** This resolves a real tension the
PRD doesn't spell out: NFR Performance says checking "must not become the bottleneck," but R3.1 says
every merge passes through verification "before being accepted" — if that meant blocking the `git
push` itself on a full test suite, push latency would be exactly the bottleneck NFR Performance
warns against. The mockup already shows the resolution: `allele-change.html`'s own change (`wt-07`)
was clearly pushed and indexed successfully — its symbol tree, diff, and conflict report all render
— and only shows "Blocked" once it's evaluated for merge, not at push time. So: a push always
succeeds and is indexed immediately (new/changed symbols, updated overlap matrix); `EnqueueMerge` is
the explicit action (or an automatic one, once a worktree reports "Ready") that triggers the AST
conflict check against `main` and every entry ahead of it in the queue, plus a Bench verification
run, both asynchronous. A merge fast-forwards into `main` only once both pass, in queue order,
matching `allele-worktrees.html`'s own footer text verbatim: "Queue merges in order, re-verified
after each."

**The pairwise overlap matrix** (`allele-worktrees.html`'s own centerpiece) is computed by diffing
every pair of currently-open branches' changed-symbol sets against their common merge base — O(n²)
pairs, fine at any concurrency level this PRD has actually discussed, revisited only if R1.3's still
unset target turns out to be very large (see [Open Questions](#open-questions)).

## Structured Verification Loops (R3)

**Verification reuses [Bench](/docs/architecture/bench-workflow-engine), Draft's existing
DAG-of-steps workflow engine — Allele does not build a second one.** R3.2 asks for verification
that's "configurable per-repository or per-path, not hard-coded into the server," which is precisely
what Bench's `Workflow`/`Step` model already is. Concretely: a repository commits a
`.allele/verify.yaml` file (versioned the same way the code it verifies is — consistent with this
repo's general config-as-code bias), written in Bench's own workflow schema; when a change enters
the merge queue, Allele calls Bench's existing `WorkflowService.TriggerRun` (not a new RPC on
Bench's side) with the change's commit SHA and changed-symbol list as trigger inputs, and polls
`GetRun` for its terminal status. `allele-change.html`'s own named steps — "Parse + type check,"
"Unit tests · 214," "Contract tests · fuse.proto," "Merge simulation with open worktrees," "Bench ·
fuse-proxy-e2e" — map directly onto Bench `Step`s; Allele's `VerificationStep` (see [Data
Model](#data-model)) is a thin, display-oriented mirror of Bench's own `StepResult`, not a
competing model. "Merge simulation with open worktrees" specifically is the AST conflict check
described above, run as one more step in the same sequence, so the UI shows one unified pass/fail
list regardless of which engine (Allele's own AST core, or a Bench-dispatched test suite) produced
each line. A failed step's detail (R3.3) is exactly what `StepResult.request`/`result`/`detail`
already carry in Bench — reused, not reinvented.

## Provenance and Permission Enforcement (R4)

- **R4.1 — commit attribution, enforced server-side.** Every git HTTP request to Allele passes
  through Fuse's `ext_authz` filter first, arriving with an `X-Authentik-Username` header the
  Authentik outpost sets and Envoy guarantees can't be client-injected (per
  [`authentication.md`](/docs/architecture/authentication#machine-to-machine-authentication)).
  Allele's receive-pack handler reads this header — never the git client's own `user.name`/
  `user.email` — and uses it as the enforced committer identity, satisfying "enforced by the server,
  not self-reported by the client" exactly as written.
- **R4.2 — per-path permission.** A new `PathPermission` record per repository (allow/deny glob
  lists, deny wins on overlap) — directly modeling `allele-change.html`'s own Permissions panel
  ("may edit: `fuse/balancer/**`, `fuse/route/**`, `fuse/config/**`" / "may not: public API in
  `draft-api`, linkage manifest"). Enforced inside the `pre-receive` hook (see
  [Decisions](#decisions)) by diffing the push's changed paths against the pushing identity's
  granted globs — a violation **rejects the push outright**, not just flags it, since this is a hard
  authorization boundary, not a conflict signal.
- **R4.3 — a durable, independently-verifiable signature for agent- and evolution-authored
  commits.** A JWT only proves who called *right now*; R4.3 asks for provenance "queryable after
  the fact," which needs something that outlives the request. For `WorktreeOwnerKind.AGENT` and
  `EVOLUTION_RUN` worktrees, Allele mints an Ed25519 keypair per identity and signs every
  **accepted merge commit itself**, server-side, using git's native SSH-signature commit format
  (supported natively by git ≥ 2.34, verifiable with `git log --show-signature` or any
  Sigstore/GitHub-style "Verified" badge, with zero Allele-specific tooling needed to check it
  later) — this is what the change-detail mockup's "signed: agent key `7F3A…C2` ✓" is. The server
  signs on the identity's behalf rather than trusting a client-provided signature, for the same
  reason R4.1's attribution is server-enforced: an agent holding its own private key would make the
  signature self-reported, which proves nothing R4.1 didn't already rule out. The key itself is
  **Allele's own**, keyed by the same `X-Authentik-Username` string already used for attribution —
  not something the Agent Service mints or needs to know about (see [Decisions](#decisions));
  minting one lazily on an identity's first push is safe, since a key alone grants no path
  permission (R4.2 is a separate, independently-granted gate). The full record — identity,
  `objective_id`/`task_id`, the `PathPermission` snapshot actually checked, the verification result,
  the signing key id — is persisted in Postgres and queryable via `GetProvenance`, independent of
  whether anyone was watching when the merge happened.
- **Humans hold their own signing key — the server never signs on a human's behalf.** Resolved
  deliberately the opposite way from agents (confirmed with the user, 2026-10-01): a human registers
  an ordinary SSH public key (the same kind every developer already has) as a `SigningKey` (see
  [Data Model](#data-model)) and signs locally with `git config gpg.format ssh` +
  `user.signingkey`, the conventional flow GitHub/GitLab already support. Allele **verifies** a
  human commit's signature — shelling out to `ssh-keygen -Y verify` against the registered key, the
  same tool and SSH-signature format used for server-side agent signing above, so no second
  verification path or crypto library is needed — rather than generating one. Signing stays
  **optional** for humans by default: an unsigned human commit is still accepted, matching how
  every other git host behaves unless a repository explicitly opts into requiring it (a small,
  later, per-repository enhancement this plan doesn't build — see [Non-Goals](#non-goals)). The
  reasoning: server-side signing is correct for agents specifically because a self-held agent key
  would prove nothing beyond the agent's own say-so — that argument doesn't extend to a human, where
  holding their own key is exactly what lets their signature mean something even if Allele's own
  server were later compromised, a real security property worth keeping rather than trading away
  for flow consistency. `allele-worktrees.html`'s `wt-02` ("Andrew") is exactly this case: a
  first-class `WorktreeOwnerKind.HUMAN` worktree, verified rather than server-signed.

## The Linkage Manifest (§7)

**`draft.linkage.yaml`, committed in each repository, is both the versioned source of truth and the
thing Allele's own generic diff core diffs — not a special case.** This is what actually delivers
R5.1 ("version-controlled using the same underlying mechanism as source... it can be branched, and
changes to it can be diffed") almost for free: because the manifest is just a file in the repo, it
goes through the identical push → index → diff pipeline as any Go or Rust file, parsed with
`tree-sitter-yaml` instead of text diff, so a key reorder or whitespace change never shows up as a
change — only a real structural edit does (an edge's `mode` changed, a node added or removed),
satisfying R5.3 directly. A typed `LinkageManifest`/`Component`/`Link` proto model exists purely as
a **read-model** the UI renders from (the graph view, the version table) — derived by parsing the
YAML after it's diffed, never a parallel store that could drift from the file itself.

**R5.4 (is a composition-mode change breaking?) gets a mechanism, and now a default too.**
`allele-linkage.html`'s own three-way radio choice ("Always breaking" / "Breaking only if the
interface changes" / "Never breaking") is modeled as a `BreakingChangePolicy` set **per
repository**, not hard-coded — which sidesteps needing one global answer to R5.4 for every
repository, since different ones can reasonably choose differently. The mockup itself already
settles the default, rather than this being a judgment call made from scratch: its radio group
ships with "Breaking only if the interface changes" pre-checked and captioned "Current setting for
this mockup," so `BREAKING_CHANGE_POLICY_INTERFACE_ONLY` is what a repository gets if it never sets
one — see [Decisions](#decisions).

## Data Model

### Proto

New package `tooling.allele.v1` (`api/tooling/allele/v1/models.proto`), following the
`models.proto`/`service.proto` split every tooling-domain API in this repo uses:

```protobuf
message Repository {
  string id             = 1;
  string name           = 2;   // "steady-bytes/draft"
  string description    = 3;
  repeated string languages = 4; // ["go","rust","proto"] -- detected, not configured
  string default_branch  = 5;    // "main"
  int64  symbol_count    = 6;    // cached from the latest AST index
  google.protobuf.Timestamp created_at = 7;
}

enum WorktreeOwnerKind {
  WORKTREE_OWNER_KIND_UNSPECIFIED   = 0;
  WORKTREE_OWNER_KIND_AGENT         = 1; // an Agent Service Executor
  WORKTREE_OWNER_KIND_HUMAN         = 2;
  WORKTREE_OWNER_KIND_EVOLUTION_RUN = 3; // a candidate branch from the separate evolution plugin
}

enum WorktreeStatus {
  WORKTREE_STATUS_UNSPECIFIED = 0;
  WORKTREE_STATUS_EDITING     = 1;
  WORKTREE_STATUS_READY       = 2; // pushed, no open conflict against the current queue order
  WORKTREE_STATUS_QUEUED      = 3;
  WORKTREE_STATUS_VERIFYING   = 4;
  WORKTREE_STATUS_BLOCKED     = 5; // real semantic conflict or failed verification
  WORKTREE_STATUS_MERGED      = 6;
}

message Worktree {
  string id              = 1;  // "wt-07" -- display id; the real git ref is branch, below
  string repository_id   = 2;
  string branch          = 3;  // refs/heads/wt-07
  string base_commit     = 4;  // merge-base sha this worktree branched from
  WorktreeOwnerKind owner_kind = 5;
  string owner_id        = 6;  // objective_id (agent), identity (human), or run_id (evolution)
  string task_label      = 7;  // "Weighted upstream pools" -- display only
  int64  symbol_count    = 8;  // symbols changed vs. base_commit, cached from the latest index
  WorktreeStatus status   = 9;
  google.protobuf.Timestamp opened_at = 10;
}

enum SymbolChangeOp {
  SYMBOL_CHANGE_OP_UNSPECIFIED       = 0;
  SYMBOL_CHANGE_OP_ADDED             = 1;
  SYMBOL_CHANGE_OP_MODIFIED          = 2;
  SYMBOL_CHANGE_OP_DELETED           = 3;
  SYMBOL_CHANGE_OP_SIGNATURE_CHANGED = 4;
}

message SymbolChange {
  string id            = 1;
  string path           = 2; // services/core/fuse/balancer/weighted.go
  string symbol         = 3; // "balancer.Weighted.Pick"
  string kind           = 4; // "method" | "type" | "field" -- open-ended, language-reported
  SymbolChangeOp op      = 5;
  string signature_before = 6; // set only when op = SIGNATURE_CHANGED
  string signature_after  = 7;
  repeated string affected_callers = 8; // fully-qualified symbols whose call sites auto-updated
}

enum ConflictKind {
  CONFLICT_KIND_UNSPECIFIED      = 0;
  CONFLICT_KIND_SAME_NODE        = 1; // both sides edit the same AST node incompatibly
  CONFLICT_KIND_BROKEN_REFERENCE = 2; // R2.2's case: a caller now reads what the other side removed
}

message ConflictReport {
  string id                = 1;
  ConflictKind kind         = 2;
  string symbol             = 3; // the symbol in THIS change that's affected
  string other_worktree_id  = 4; // "wt-04"
  string other_symbol       = 5; // route.Upstream.Address
  string explanation        = 6; // "wt-04 removes the field this change starts reading..."
  bool   would_textually_merge = 7; // true = "a line-based merge would succeed" -- the whole point
}

message Change {
  string id             = 1; // "CH-219"
  string repository_id  = 2;
  string worktree_id    = 3;
  string title           = 4;
  repeated SymbolChange symbols = 5;
  int64  files_changed   = 6;
  int64  lines_added     = 7;
  int64  lines_removed   = 8;
  repeated ConflictReport conflicts = 9;
  Provenance provenance   = 10;
  google.protobuf.Timestamp opened_at = 11;
}

enum VerificationStatus {
  VERIFICATION_STATUS_UNSPECIFIED = 0;
  VERIFICATION_STATUS_WAITING     = 1;
  VERIFICATION_STATUS_RUNNING     = 2;
  VERIFICATION_STATUS_PASSED      = 3;
  VERIFICATION_STATUS_FAILED      = 4;
}

message VerificationStep {
  string name    = 1; // "Parse + type check", "Unit tests · 214", "Merge simulation..."
  VerificationStatus status = 2;
  string detail   = 3; // failure detail or timing, free text
  int64  duration_ms = 4;
}

message VerificationRun {
  string id            = 1;
  string change_id      = 2;
  string bench_run_id   = 3; // foreign reference into Bench's own Run -- see Decisions
  repeated VerificationStep steps = 4;
  VerificationStatus status = 5;
  google.protobuf.Timestamp started_at  = 6;
  google.protobuf.Timestamp finished_at = 7;
}

message Provenance {
  string change_id        = 1;
  string identity          = 2; // X-Authentik-Username at push time -- the enforced "who"
  WorktreeOwnerKind owner_kind = 3;
  string objective_id      = 4; // set when owner_kind = AGENT; foreign id, not a local join
  string task_id           = 5;
  repeated string permitted_paths = 6; // the PathPermission globs checked, snapshotted
  bool   permission_violated = 7;      // always false for an accepted change
  string signature_key_id   = 8;       // which identity's Ed25519 key signed the merge commit
  string commit_sha         = 9;       // the signed merge commit, once merged
  google.protobuf.Timestamp recorded_at = 10;
}

message PathPermission {
  string id               = 1;
  string repository_id     = 2;
  string identity_pattern  = 3; // matches X-Authentik-Username, exact string for now
  repeated string allow_globs = 4; // ["fuse/balancer/**", "fuse/route/**"]
  repeated string deny_globs  = 5; // ["draft-api/**", "draft.linkage.yaml"] -- deny wins
}

message SigningKey {
  string id               = 1;
  string identity          = 2; // the X-Authentik-Username this key verifies commits for
  string public_key        = 3; // SSH public key, authorized_keys format (git's gpg.format=ssh)
  google.protobuf.Timestamp registered_at = 4;
  google.protobuf.Timestamp revoked_at    = 5; // unset while active
}

enum LinkMode {
  LINK_MODE_UNSPECIFIED     = 0;
  LINK_MODE_STATIC          = 1;
  LINK_MODE_RUNTIME_SERVICE = 2;
  LINK_MODE_SIDECAR         = 3;
}

message Link {
  string target    = 1; // the component this link attaches to
  LinkMode mode     = 2;
  string transport  = 3; // "otlp/grpc . unix socket" -- free text, mode-dependent
}

message Component {
  string name   = 1;
  string version = 2;
  repeated Link links = 3;
}

enum BreakingChangePolicy {
  BREAKING_CHANGE_POLICY_UNSPECIFIED    = 0;
  BREAKING_CHANGE_POLICY_ALWAYS         = 1; // any mode change is breaking
  BREAKING_CHANGE_POLICY_INTERFACE_ONLY = 2; // breaking only if the interface itself changed
  BREAKING_CHANGE_POLICY_NEVER          = 3;
}

message LinkageManifest {
  string repository_id = 1;
  string commit_sha     = 2; // the draft.linkage.yaml blob this was parsed from
  repeated Component components = 3;
  BreakingChangePolicy policy = 4;
}
```

### Postgres, not Blueprint KV

Every message above is stored in Postgres via `bun`, following the exact `pkg/repositories/postgres`
pattern Relay and Bench both already established, registered with chassis the ordinary way
(`WithRepository`) — chosen for the same reason Relay chose it over Lineman's Blueprint-only default:
the overlap matrix, path-glob matching, and multi-table provenance queries are real relational
access patterns a raft-replicated KV store answers poorly. Messages with a `google.protobuf.Timestamp`
field (`Repository`, `Worktree`, `Change`, `Provenance`) use a dedicated row struct + conversion
functions rather than the generated type directly as a bun model, and any `jsonb` column (symbol
lists, step lists) uses `json.RawMessage`, not `[]byte` — both following the already-documented
precedent and gotcha from [Relay's Phase 3](/docs/architecture/relay-implementation-plan#implementation-plan)
and [Bench's Phase 1](/docs/architecture/bench-workflow-engine#implementation-plan) own phase notes,
so this plan doesn't rediscover either. Bare git repositories themselves live on **local disk**, one active
Allele instance, not Postgres and not a new object-store service — see
[Decisions](#decisions).

## RPC Interface

`api/tooling/allele/v1/service.proto`:

```protobuf
service AlleleService {
    // Repositories
    rpc ListRepositories(ListRepositoriesRequest) returns (ListRepositoriesResponse) {}
    rpc CreateRepository(CreateRepositoryRequest) returns (Repository) {}
    rpc GetRepository(GetRepositoryRequest) returns (Repository) {}

    // Worktrees -- opening one is observed, not commanded: the first push of a new
    // branch is what creates its row. GetWorktreeOverlap backs the pairwise matrix.
    rpc ListWorktrees(ListWorktreesRequest) returns (ListWorktreesResponse) {}
    rpc GetWorktreeOverlap(GetWorktreeOverlapRequest) returns (GetWorktreeOverlapResponse) {}
    rpc CloseWorktree(CloseWorktreeRequest) returns (CloseWorktreeResponse) {} // abandon, no merge

    // Changes -- a Change is the live, continuously-reindexed diff of one worktree's
    // branch against its base. EnqueueMerge enters the merge queue; it does not merge
    // immediately -- see Agent worktrees are branches, and verification is queue-gated.
    rpc ListChanges(ListChangesRequest) returns (ListChangesResponse) {}
    rpc GetChange(GetChangeRequest) returns (Change) {}
    rpc RebaseChange(RebaseChangeRequest) returns (Change) {}
    rpc EnqueueMerge(EnqueueMergeRequest) returns (MergeQueueEntry) {}

    // Merge queue
    rpc ListMergeQueue(ListMergeQueueRequest) returns (ListMergeQueueResponse) {}
    rpc HoldMergeQueueEntry(HoldMergeQueueEntryRequest) returns (MergeQueueEntry) {}
    rpc ResumeMergeQueueEntry(ResumeMergeQueueEntryRequest) returns (MergeQueueEntry) {}

    // Live feed -- backs the symbol-changes drawer and the queue's own live status.
    // Dedicated streaming RPC, not a Catalyst Consume subscription -- same reasoning
    // as Relay's WatchTranscript; see Decisions.
    rpc WatchChanges(WatchChangesRequest) returns (stream SymbolChangeEvent) {}

    // Provenance
    rpc GetProvenance(GetProvenanceRequest) returns (Provenance) {}
    rpc ListPathPermissions(ListPathPermissionsRequest) returns (ListPathPermissionsResponse) {}
    rpc SetPathPermissions(SetPathPermissionsRequest) returns (SetPathPermissionsResponse) {}

    // Signing keys -- humans register their own (see Decisions); agents/evolution runs never
    // call these, since Allele mints and holds their keys itself.
    rpc RegisterSigningKey(RegisterSigningKeyRequest) returns (SigningKey) {}
    rpc ListSigningKeys(ListSigningKeysRequest) returns (ListSigningKeysResponse) {}
    rpc RevokeSigningKey(RevokeSigningKeyRequest) returns (RevokeSigningKeyResponse) {}

    // Linkage manifest
    rpc GetLinkageManifest(GetLinkageManifestRequest) returns (LinkageManifest) {}
    rpc DiffLinkageManifest(DiffLinkageManifestRequest) returns (DiffLinkageManifestResponse) {}
    rpc SetBreakingChangePolicy(SetBreakingChangePolicyRequest) returns (LinkageManifest) {}
    rpc ApproveLinkageChange(ApproveLinkageChangeRequest) returns (ApproveLinkageChangeResponse) {}
}

message SymbolChangeEvent {
    string repository_id = 1;
    string worktree_id    = 2;
    SymbolChange change    = 3;
    google.protobuf.Timestamp at = 4;
}
```

**The actual git read/write path is deliberately not part of `AlleleService`.** `git clone`/`fetch`/
`push` speak git's own smart-HTTP protocol — plain HTTP, not Connect-framed requests — against:

```
GET  /{repo}/info/refs?service=git-upload-pack|git-receive-pack
POST /{repo}/git-upload-pack
POST /{repo}/git-receive-pack
```

mounted via `chassis.AddHandler`, the same split Bench's webhook receiver established between raw
HTTP and its ordinary RPC surface. `AlleleService` is how the UI and other Draft services read
Allele's *derived* state (worktrees, diffs, provenance); the git endpoints are how an agent's actual
`git` client reads and writes *repository* state. Both sit behind the same Fuse route and the same
`ext_authz` check — see [Provenance and Permission Enforcement](#provenance-and-permission-enforcement-r4).

## Telemetry

Every mutating RPC opens its own `chassis.StartSpan` child span, business attributes set before
`span.End`, matching `crud`/Relay/Lineman exactly:

| RPC | Business attributes |
|---|---|
| `EnqueueMerge` | `change_id`, `worktree_id`, `repository_id` |
| `GetWorktreeOverlap` | `repository_id`, `pair_count`, `conflict_count` |
| `SetPathPermissions` | `repository_id`, `identity_pattern` |
| `ApproveLinkageChange` | `repository_id`, `policy` |

Catalyst CloudEvents, following the `source` prefix-filtering convention [the Agent Service already
established](/docs/architecture/agents#cloudevent-mapping) (`agent.AgentEvent/{objective_id}`
exact-match or `agent.AgentEvent/` prefix-match) so cluster consumers can subscribe at either
granularity without a second convention to learn:

| CloudEvent `type` | `source` | Fires when |
|---|---|---|
| `tooling.allele.v1.SymbolChanged` | `allele.SymbolChange/{repository_id}` | Every symbol-level change indexed from a push |
| `tooling.allele.v1.ConflictDetected` | `allele.Change/{change_id}` | A merge-queue AST check finds a real conflict (R2.2) |
| `tooling.allele.v1.VerificationCompleted` | `allele.Change/{change_id}` | A Bench-backed verification run reaches a terminal status |
| `tooling.allele.v1.LinkageChanged` | `allele.LinkageManifest/{repository_id}` | `draft.linkage.yaml` structurally changes on any branch |
| `tooling.allele.v1.MergeCompleted` | `allele.Change/{change_id}` | A queued change is fast-forwarded into `main` |

## Decisions

**Smart-HTTP(S), not SSH, as the only git transport in this plan — a deliberate departure from the
mockups' own `git@allele.draft.localhost` copy, named explicitly rather than silently dropped.**
Every other piece of this cluster is HTTP-native: Fuse/Envoy routing, `ext_authz`, Connect-RPC. Smart
HTTP gets Allele request-time auth, routing, and TLS entirely for free from infrastructure that
already exists (an M2M JWT as an HTTP Basic/bearer credential, exactly how GitHub/GitLab/Bitbucket
all already support HTTPS+PAT remotes). SSH would mean Allele running its own listener, speaking the
git-over-SSH pack protocol itself, and doing public-key-to-identity resolution entirely independent
of Authentik/Fuse — a materially larger, self-contained undertaking for a transport that's
functionally equivalent to HTTPS for every operation Allele needs. SSH support is left a disclosed
non-goal (see [Non-Goals](#non-goals)), addable later without changing anything above the transport
layer.

**`git http-backend` is wrapped, not reimplemented or replaced by a pure-Go git stack.** Git ships
`git-http-backend` specifically to be wrapped by a CGI-style handler (Go's `net/http/cgi`) — the same
approach Gitea and Gogs both took for their own smart-HTTP serving. Allele's `pre-receive` hook
(installed by Allele into every bare repo it creates) is where the AST conflict/permission/signing
pipeline actually runs, called back into Allele's own running process — the standard mechanism
GitHub/GitLab/Gitea all use internally for branch-protection-style checks, not a novel integration
point this plan invents.

**A single active Allele instance owns the bare repositories on local disk — no multi-instance
replication of git storage in this plan.** Unlike Relay (where multiple instances are forced by
physically-attached hardware), nothing about serving git requires more than one writer. This matches
how Postgres itself, and every other single-writer stateful piece of this cluster outside Blueprint's
own raft, already starts — simplest possible, revisited only if availability requirements demand it
(see [Non-Goals](#non-goals)).

**Verification reuses Bench; Allele builds no second workflow/DAG engine.** See [Structured
Verification Loops](#structured-verification-loops-r3) — this is the single biggest piece of scope
this plan avoids by not duplicating infrastructure that already exists and already has exactly the
right shape (per-repository, per-path configurable steps).

**`WatchChanges` is a dedicated streaming RPC, not a Catalyst `Consume` subscription**, for the same
reason [Relay's `WatchTranscript`
is](/docs/architecture/relay-implementation-plan#decisions): a live UI panel needs reliable,
single-recipient delivery, and [Catalyst's fan-out bug is already
fixed](/docs/architecture/core-services#known-issues), so this is a genuine simplicity choice, not a
bug-driven workaround — the `allele.symbol.*` CloudEvents on Catalyst remain the channel for *other*
cluster consumers (the Planner/OODA loop, per PRD §9), independent of what Allele's own UI watches.

**The linkage manifest's version history is just `draft.linkage.yaml`'s own git history — no
separate store.** This resolves the PRD's own persistence-model open question (§13) for Allele
specifically: the question was "whether linkage manifest state and ancestry should live in Postgres
or ClickHouse," and the answer is neither, because it's already versioned the moment it's a file in
a git repository Allele already hosts. The PRD notes this question "may apply ... to the separate
evolution-plugin work too" — that part stays open, since it's not this plan's component.

**Per-repository `BreakingChangePolicy` defaults to `INTERFACE_ONLY`.** Sourced directly from
`allele-linkage.html`'s own pre-checked radio option — see [The Linkage
Manifest](#the-linkage-manifest-7) for the evidence. A repository can still set `ALWAYS` or `NEVER`
explicitly; this is only what an unset one gets.

**The pairwise overlap matrix ships as plain O(n²) comparison, isolated behind one function
boundary so it can become incremental later without `WorktreeService` callers changing.** Confirmed
with the user 2026-10-01: no specific concurrency target is set (R1.3 stays open — see [Open
Questions](#open-questions)), and the design posture is to build the simple version now rather than
guess at an optimization a real number may never require, without painting the implementation into
a corner if it turns out to.

**Allele mints and holds its own identity → Ed25519-key table for agent/evolution signing — no
change to `agents.md` needed.** See [Provenance and Permission
Enforcement](#provenance-and-permission-enforcement-r4)'s R4.3: the private key has to stay
server-side regardless of which subsystem conceptually "owns" the identity, since a self-held agent
key would make R4.1's own server-enforcement requirement meaningless. Resolved by tracing through
what R4.1 already requires, not a decision needing sign-off from a separate owner.

**Humans hold their own SSH signing key; only agents and evolution runs get server-side signing.**
Confirmed with the user 2026-10-01 — see [Provenance and Permission
Enforcement](#provenance-and-permission-enforcement-r4)'s R4.3 for the full reasoning and the new
`SigningKey` record this adds to the data model.

**The ephemeral verification checkout needs no sandbox decision of its own.** See [Agent worktrees
are branches](#agent-worktrees-are-branches-and-verification-is-queue-gated-not-push-gated) — the
AST merge-simulation step only parses, never executes, so it runs safely in-process on Allele;
actually executing untrusted build/test commands is entirely Bench's existing responsibility, not
new scope this plan introduces.

## Non-Goals

- **No genetic-algorithm/evolution engine.** See [Relationship to the Agent Service, the Evolution
  Plugin, and Auth](#relationship-to-the-agent-service-the-evolution-plugin-and-auth) — Allele
  supports it generically as a client, it doesn't build it.
- **No second workflow/DAG engine for verification.** Reuses Bench entirely; see Decisions.
- **No SSH git transport in this plan.** Smart-HTTP(S) only; see Decisions. A real, designed-for gap
  the mockups' own copy implies, named explicitly rather than silently dropped.
- **No multi-instance/replicated Allele deployment.** One active instance owns the bare repos on
  local disk; see Decisions.
- **No literal per-agent, persistently checked-out working directory on the server.** A "worktree"
  is a branch plus an index entry; an actual checkout exists only transiently, during verification.
- **No LSP-based semantic conflict detection (R2.5) in this plan's phases.** Explicitly Phase 3 in
  the PRD's own phasing (§12) and "not a blocker" there; carried into this plan's [final
  phase](#implementation-plan) in name only.
- **No bundled, Allele-specific client tooling.** An agent is whatever process is already driving
  `git` against the Agent Service's Executor — Allele expects an ordinary git client against its
  smart-HTTP remote, not a custom CLI or library.
- **No per-repository "require signed commits" enforcement toggle.** A human's commit signature is
  verified when present but optional by default in every phase here, matching how most git hosts
  behave out of the box; a GitHub-style branch-protection switch making it mandatory is a small,
  later, additive enhancement, not attempted now.
- **No GitOps/deployment decisioning, no new VCS protocol, no full program verification, and no
  day-one language support beyond Go/Rust/(best-effort proto)** — restated directly from PRD §4 and
  §8.4; this plan doesn't relitigate them.

## Implementation Plan

The PRD's own §12 phasing (Phase 0–3) is intentionally coarse — "a proposed sequencing... not a
committed schedule." The checklist below breaks it into the same fine-grained,
independently-shippable slices every other plan in this repo uses; each phase names which PRD phase
it falls under. Nothing below is started.

- [x] **1. Scaffolding.** `services/tooling/allele`: `go.mod`, `main.go`, `config.yaml` (port 9311 —
  9310 was the next free *tooling* port after Relay's `whisper.cpp` sidecar at 9309, but it
  collided with an unrelated process already bound to it on the dev machine, confirmed via `lsof`),
  following the
  `chassis.New(logger).Register(...).WithRepository(db).WithRPCHandler(...).WithRoute(...).Start()`
  shape every tooling service uses. Postgres via `bun`. Registers with Blueprint. _(Prerequisite to
  PRD Phase 0.)_ Deliverable: a service that starts, registers, and connects to Postgres, and does
  nothing else yet. _(completed 2026-10-01 — `go build`/`go vet`/`gofmt` all clean. `go.mod` needs
  the same local `replace` directives for `api`/`pkg/chassis`/`pkg/loggers` every other tooling
  service uses, for the same reason (`chassis.Effect`, which `WithRepository` is built on, isn't in
  a published `pkg/chassis` release yet). Own `allele`/`allele` Postgres role+database created (both
  scripts' provisioning blocks, and the live already-running container — see below). Both
  `scripts/run-local.sh` and `scripts/run-local-watch.sh` updated in parallel (port-check lists,
  Postgres provisioning, build step, `start_bg`/`start_watched` entry, summary printout), syntax-checked
  with `bash -n`; `run-local-watch.sh`'s port-check unconditionally kills whatever holds a listed
  port (`free_process_port`), the same treatment already applied to every other service's port
  there — worth knowing given 9310's live collision just above, but not a new risk this phase
  introduces. Live-verified against the real already-running local stack, not just built: started
  the binary directly (Blueprint/Postgres both already up), confirmed via a scratch Connect client
  against Blueprint's real `ServiceDiscoveryService.Query` that `allele` is registered with
  `state=PROCESS_RUNNING` at `localhost:9311`, and confirmed a real idle connection from the
  `allele` Postgres role in `pg_stat_activity` — the same two checks Relay's own Phase 1 note used
  to distinguish a real connection from a merely-valid config. A stale `PROCESS_STARTING` entry
  with no IP is left in Blueprint's registry from an earlier crashed attempt on the colliding port
  9310 (the process never reached a clean shutdown to deregister) — harmless, and not cleaned up
  by hand, since poking at Blueprint's registry state beyond normal register/deregister wasn't this
  phase's job. Left the pre-existing uncommitted changes already in the working tree (Relay's own
  `api/build.rs`/`api/src/hook/mod.rs`/`api/src/proto/mod.rs`/`run-local-watch.sh`/`mockups/index.html`
  edits) untouched and layered Allele's own additions alongside them; nothing was committed.)_
- [x] **2. Proto.** `api/tooling/allele/v1/{models,service}.proto` per [Data Model](#data-model) and
  [RPC Interface](#rpc-interface). Regenerate (`dctl api build`), remembering to add
  `tooling/allele/v1/` to `api/build.rs`'s `proto_dirs` list — the exact gotcha both Lineman's and
  Relay's own Phase 2 hit. _(Prerequisite to PRD Phase 0.)_ Deliverable: generated Go/Rust/TS types
  compile; no behavior change yet. _(completed 2026-10-01 — `buf build`/`buf lint` clean on the new
  package before generating anything. Used `buf generate --path tooling/allele/v1` scoped to just
  this package rather than a full `dctl api build`, specifically to avoid regenerating every other
  already-committed proto package's output over version/formatting drift as a side effect of adding
  one new one — all plugins (`protoc-gen-go`, `-connect-go`, `-validate`, `-gotag`) were already
  installed locally, so this needed no Docker. The plan's own RPC Interface sketch left most
  request/response shapes implicit; filled them in as standard List/Get-shaped messages during this
  phase, plus two additions the sketch didn't name: `MergeQueueEntry` (the merge queue's own row,
  not stored independently of the queue the way `Change`/`Worktree` are) and `OverlapPair`/
  `LinkageManifestChange` (typed shapes for the overlap matrix and the manifest diff view,
  respectively) — the same kind of gap-filling Relay's own Phase 2 (`SearchHit`) and Bench's Phase 2
  (`CreateWorkflow`/`UpdateWorkflow`) hit. Proactively added `./tooling/allele/v1/` to
  `api/build.rs`'s `proto_dirs` (confirmed this is the right file: Lineman/Relay's own Phase 2 notes
  both hit this exact gotcha). `go build ./...`/`go vet` clean on the whole `api` Go module
  afterward, not just the new package. **`api/src/proto/mod.rs` and `api/src/hook/mod.rs` are
  themselves generated** — `build.rs` rewrites both from scratch on every `cargo build` by scanning
  its own output directories, confirmed by reading `build.rs` itself before touching either file by
  hand (would have violated CLAUDE.md's "never hand-edit generated files" even though the content
  would have looked identical) — so Rust registration needed no manual mod.rs edit at all, just
  `cargo check` on the `api` crate, which came back clean and produced
  `src/proto/tooling.allele.v1.rs` / `src/hook/tooling.allele.v1.dx.rs` correctly self-registered.
  Web/TS codegen fails the same pre-existing way it does for every other tooling-domain proto
  (missing local `protoc-gen-es`/`-connect-es`/`-connect-query` on `$PATH` — confirmed by attempting
  it and seeing the identical plugin-not-found error Relay's own Phase 2 note already documented for
  Lineman's `workflow.v1`/`plugin_catalog.v1`) — not something this phase broke.)_
- [x] **3. Git hosting, no diffing yet.** Wrap `git http-backend` behind `chassis.AddHandler` per
  [Decisions](#decisions); `CreateRepository`/`ListRepositories` managing bare repos under a
  configured local directory; Fuse route for the git paths and for `AlleleService`'s own prefix.
  _(Prerequisite to PRD Phase 0 — proves the hosting plumbing before any AST logic exists, the same
  "prove the simple path first" ordering Relay used for browser-only audio before native hardware.)_
  Deliverable: `git clone`/`push` against a real Allele-hosted repository succeeds end to end, with
  zero structural analysis involved. _(completed 2026-10-01 — `model.go` (`repositoryRow`, bun's
  native Postgres array support for `languages` — a first in this repo, every other repeated-string
  field elsewhere went into jsonb instead), `store.go` (`CreateRepository` does both halves: the
  Postgres row and a real `git init --bare --initial-branch=main` + `git config
  http.receivepack true` on local disk, rolling the directory back if the DB insert fails),
  `githttp.go` (`net/http/cgi` wrapping `git http-backend`, gated by a real registry check —
  `getRepositoryByName` — before ever touching the subprocess, registered at `/` as the catch-all
  behind `AlleleService`'s own more specific Connect path per `http.ServeMux`'s longest-match-wins),
  `rpc.go` (all 24 `AlleleServiceHandler` methods; 3 real, 21 `CodeUnimplemented` naming the phase
  that owns each one). `go build`/`go vet`/`gofmt` clean. **Live-verified against the real running
  local stack, not just built**: `CreateRepository` over Connect's plain JSON protocol (no scratch
  client needed this time) produced a real UUID'd row, a real bare repo on disk with
  `http.receivepack=true`, and a matching Postgres row, confirmed three ways (direct `ls`/`git
  config`, a raw `psql` query, and `ListRepositories`). Then the actual deliverable, for real: `git
  clone` (empty-repo warning, as expected) → real commit → `git push` (`[new branch] main -> main`)
  → a completely fresh `git clone` elsewhere recovered the identical commit hash and file content —
  proof the push landed server-side, not just that the client reported success.
  <br><br>**Known issue found, not fixed, and confirmed unrelated to this phase's own code**: Fuse
  route registration (`WithRoute`) doesn't actually work on this dev stack right now — not just for
  Allele, but for every route-registering service (Relay, Bench, Blueprint's own UI). Chasing it
  surfaced and then made worse a separate, pre-existing problem: Blueprint's raft cluster had 4 of
  5 nodes already down (unrelated to this phase), quietly papered over because the sole running
  node was already an elected leader that needed no new election — until a Fuse restart forced it
  to call one it couldn't win alone. Recovered by restarting the missing nodes via
  `run-local-watch.sh`'s own already-established join sequence (not an improvised equivalent) and
  confirming a clean leader election in the raft log. The user then did an independent fresh
  restart of the whole stack; Fuse routing for every service, Allele included, still 404s afterward
  with zero trace of the `AddRoute` call ever reaching Fuse's own handler (not even a logged
  failure) — ruling out anything this phase's restarts caused. This does not touch the phase's own
  deliverable, which was fully verified above via direct access throughout; it's logged here as a
  known, pre-existing Fuse issue worth its own dedicated look, not blocking Phase 4.)_
- [x] **4. Tree-sitter parsing integration.** `go-tree-sitter` plus the Go and Rust grammars wired
  in; vet a `.proto` grammar and decide structural-vs-text-diff for it per
  [Decisions](#ast-level-structural-diff-and-3-way-merge-the-hard-part). _(PRD Phase 0.)_
  Deliverable: an internal tool that parses a real file from this repo and prints its tree; no
  diffing yet. _(completed 2026-10-01 — new `parsing` package (`Language`, `LanguageForPath`,
  `Parse`), and `cmd/parse` (`go run ./cmd/parse <file>`), the "internal tool" itself, matching
  Relay's own `cmd/list-devices` precedent for a standalone binary that verifies one integration
  independent of the full service. Confirmed exact package paths and APIs via direct lookup rather
  than memory before writing anything (`github.com/tree-sitter/go-tree-sitter` v0.25.0;
  `tree-sitter-go`/`tree-sitter-rust`'s own `bindings/go` subpackages, the now-standard convention
  every official grammar ships). **The `.proto` grammar vetting the plan called for was run for
  real, not assumed**: `coder3101/tree-sitter-proto` (chosen over `mitchellh/tree-sitter-proto`,
  which has no published Go bindings on pkg.go.dev) parsed this repo's own real
  `models.proto`/`service.proto` — not synthetic fixtures — with **zero `ERROR` or `MISSING`
  nodes**, and correctly distinguishes real semantic node kinds (`message`, `message_body`, `enum`,
  `enum_field`, `field`, `field_number`, `option`), not just an absence of catastrophic failure.
  The disclosed text-diff fallback for `.proto` isn't needed — removed the provisional language
  from `parsing.go` once the real result was in. Also parsed real Go (`pkg/chassis/builder.go`,
  442 lines) and Rust (`tools/dioxus-grpc/src/lib.rs`) files from this repo with zero errors. One
  dependency-resolution note, not a problem: adding `coder3101/tree-sitter-proto` bumped go.mod's
  own `go` directive from 1.24.0 to 1.26.0 (that module's own requirement) — Go's toolchain manager
  fetched 1.26.8 automatically, the same auto-fetch this plan's Phase 1 note already anticipated.
  `go build`/`go vet`/`gofmt` clean.)_
- [x] **5. Two-way structural diff + `SymbolChange` extraction.** The GumTree-style matching core
  against `go-tree-sitter`'s tree shape; `GetChange` returns a real, populated symbol tree (additions,
  modifications, deletions, signature changes) for a pushed branch against its base. _(PRD Phase
  0.)_ Deliverable: `allele-change.html`'s own symbol tree and diff view render against a real
  change, conflict detection not yet included. _(completed 2026-10-01 — **scope call, stated
  explicitly rather than silently narrowed**: this phase implements symbol-level diffing
  (name-based matching between an old/new version of one file, direct text/signature comparison),
  not generic node-level GumTree matching. Nothing at symbol-change granularity needs node-level
  matching — that's genuinely Phase 6's own job, where disjoint-sub-field reasoning for 3-way merge
  actually requires it; building it speculatively here, ahead of Phase 6's concrete requirements,
  would mean guessing at a shape Phase 6 would likely revise anyway. New `diff` package
  (`symbols.go`'s per-language extraction, confirmed against real tree-sitter node kinds/field
  names via a probe file through `cmd/parse` before writing any extraction code — Go/Rust label
  fields (`name:`, `receiver:`, `parameters:`, `result:`/`return_type:`); the proto grammar labels
  none at all, confirmed the same way, so its extraction is positional instead, not an oversight;
  `diff.go`'s `SymbolChanges` entry point) plus `cmd/diff`, the internal tool itself, operating
  directly on two real git refs rather than against a persisted `Change` — nothing creates one yet,
  since worktree tracking is Phase 7's job; `GetChange`'s own RPC wiring to real data is correctly
  deferred to Phase 7, once there's a real worktree/base pair for it to resolve.
  <br><br>**A real bug, found live and fixed, not just a clean run to report**: the first version
  kept a `*tree_sitter.Node` on `Symbol` and called `.Utf8Text()` on it later from `compareSymbol`,
  after the source tree had already been closed (`symbolsFor`'s own `defer tree.Close()`) — a
  textbook cgo use-after-free, and it didn't just misbehave, it crashed: a real `SIGSEGV` inside
  `_Cfunc_ts_node_end_byte`, reproduced on the very first real end-to-end run against a real git
  repo, not caught by `go build`/`go vet`/`gofmt` (all clean) or by Phase 4's own grammar checks.
  Fixed by extracting each symbol's text eagerly, while its tree is still open, onto a plain `Text
  string` field — `Symbol` now holds no node pointer at all, closing off the whole class of bug
  rather than documenting a tree-lifetime rule future callers would have to remember.
  <br><br>**Live-verified against a real git repository** (three real commits, `cmd/diff` run
  directly against real commit SHAs, not fixtures): a Go method signature change
  (`(u *Upstream) Endpoint` → `(u *Upstream, h *HealthWindow) (Endpoint, error)`) correctly
  classified `SIGNATURE_CHANGED` with the exact before/after text; a new type and a new method
  correctly `ADDED`; a later commit removing a function correctly and *only* reported that
  function as `DELETED`; a Rust method signature change and a new enum classified correctly the
  same way; a proto message's new field correctly fell through to `MODIFIED` (messages have no
  separate "signature" the way a function does) and a new proto enum correctly `ADDED`. Just as
  important as what showed up: two functions deliberately left byte-identical across every commit
  (`Rebuild`, `helperStaysSame`) never appeared in any run's output — confirming unchanged symbols
  are genuinely suppressed, not just that changes are detected. `go build`/`go vet`/`gofmt` clean.)_
- [x] **6. Three-way merge + conflict detection (R2.1–R2.3).** The per-node-kind
  disjoint-sub-field merge table; auto-merge vs. flag logic; `ConflictReport` generation with a real
  structural explanation. Deliverable: the PRD's own worked examples both behave correctly end to
  end — two unrelated same-line parameter renames auto-merge; the `wt-04`/`wt-07`
  `route.Upstream.Address` removal is correctly flagged with the exact explanation shape
  `allele-change.html` shows. _(PRD Phase 0 complete at this point: "False-conflict elimination
  (R2.1, R2.2) working end to end.")_ _(completed 2026-10-02 — this phase actually needs two
  structurally different mechanisms, not one, worth separating clearly: **same-node reconciliation**
  (R2.1/R2.2's parameter-rename-vs-retype case — both sides touch the *same* symbol) and
  **cross-symbol broken-reference detection** (the `wt-04`/`wt-07` case — two *different* symbols,
  often different files, where one side's removal breaks what the other side's independent change
  now reads). New `merge` package: `slots.go` (re-parses a symbol's own already-extracted `Text` in
  isolation — confirmed empirically that a `method_declaration`'s full text re-parses cleanly
  standalone with no prefix, while a bare `type_spec`'s text does not and needs `"type "`
  prepended, since Phase 5 stores the spec alone, not its parent declaration — to decompose Go
  function parameters positionally and struct fields by name into `NamedTypedSlot`s, confirmed
  against a real two-parameter, two-field probe before writing any decomposition code);
  `reconcile.go` (`ReconcileFunction`/`ReconcileType`: a slot both sides changed *differently* is a
  conflict named precisely by which slot and which two values, R2.3; disjoint slots auto-merge,
  with no `ConflictReport` produced at all for a clean merge — there's nothing to explain);
  `references.go` (`RemovedStructFields` + `ReferencedFields`, walking every descendant of a
  symbol's own re-parsed text for a `selector_expression`, not just its top-level children, so a
  field read three statements deep is found the same as one on the first line); plus `cmd/merge`,
  the internal tool, and a new shared `gitutil` package (`ChangedFiles`/`ShowFile`/`MergeBase`)
  factored out of `cmd/diff` before `cmd/merge` could become a second, duplicate copy of the same
  git plumbing.
  <br><br>**A real, explicit scope call, not a silent gap**: sub-field decomposition
  (`slots.go`) is Go-only. Rust's and proto's own parameter/field shapes have different field
  names (confirmed different from Go's during Phase 5's own probing) and would need their own
  extraction work — real effort, not a few-line variation — and the PRD's own R2.2 worked examples
  are both Go. Left for whenever Rust/proto need the same reconciliation, not guessed at here.
  Symbol kinds this phase doesn't decompose (`struct`/`enum`/`message` — Rust and proto's own type
  kinds) fall through to being treated conservatively: both sides touching one is a review-worthy
  case today, not silently auto-merged or crashed on.
  <br><br>**The broken-reference check is a disclosed syntactic heuristic, stated plainly because
  it's a real, known limitation, not an oversight**: it matches a `.FieldName` selector by bare
  name only, never confirming the receiver is actually of the type that lost the field — a
  same-named field on an unrelated struct would false-positive, and a field reached through an
  intermediate variable the AST doesn't make locally obvious would false-negative. Real type-aware
  resolution is explicitly R2.5 (Phase 13, LSP-based — `gopls`/`rust-analyzer` actually know a
  selector's receiver type); this phase's job was proving the *mechanism* end-to-end against the
  PRD's own concrete case, not building semantic conflict detection early. Also scoped to each
  side's own *changed* surface (symbols that side's own two-way diff actually flagged), not the
  whole repository — an unchanged function that already read a since-removed field, untouched by
  either side's own edits, isn't caught; that broader check is a bigger feature (effectively a
  standing reference index) better suited to once Phase 13's LSP integration exists, not a scoped
  gap worth closing with a cruder tool first.
  <br><br>**Two edge cases handled beyond the plan's own two named examples, found while building
  this, not after**: both sides independently adding a same-named symbol (auto-merges if identical,
  conflicts if not — an "add/add" case parameter rename/retype doesn't cover) and both sides
  changing a function's parameter *count* (treated as a conflict outright — "the Nth parameter"
  stops being a meaningful shared identity once the lists disagree on length, and guessing the
  mapping is exactly the kind of semantic judgment call this phase doesn't make).
  <br><br>**Not attempted here, on purpose**: synthesizing the actual merged source text for an
  auto-merge decision. This phase's job is correctly *classifying* mergeable-vs-conflicting with a
  real explanation, per the deliverable's own wording ("auto-merge" / "correctly flagged") — not
  producing a byte-perfect merged file, which more naturally belongs to Phase 7's actual
  merge-queue execution, once there's a real merge commit being constructed to write text into.
  <br><br>**Live-verified against four real scenarios in a real git repository**, not fixtures: (1)
  the PRD's own primary example — one side renames a parameter, the other retypes the same
  parameter — correctly auto-merged with zero conflicts; (2) a negative control for (1) — both
  sides rename the *same* parameter to *different* names — correctly conflicted, explanation
  naming the exact parameter and both values, `would_textually_merge: false`; (3) the `wt-04`/
  `wt-07` scenario rebuilt exactly (a struct field removed on one side; a function on the
  independent other side starts reading that exact field) — correctly flagged
  `CONFLICT_KIND_BROKEN_REFERENCE` with the explanation "theirs removes the field
  Upstream.Address that Pick starts reading" and `would_textually_merge: true`, both matching the
  mockup's own language almost verbatim; (4) a negative control for the whole pipeline — unrelated
  changes to two different functions in the same file — correctly produced zero conflicts and zero
  auto-merges (neither symbol was touched by both sides, so neither check should fire at all).
  `go build`/`go vet`/`gofmt` clean throughout.)_
- [x] **7. Worktrees, overlap matrix, and the merge queue (R1).** `WorktreeService` behaviors, the
  push-is-always-accepted / verification-is-queue-gated split from [Agent worktrees are
  branches](#agent-worktrees-are-branches-and-verification-is-queue-gated-not-push-gated), and
  pairwise overlap computation as plain O(n²) behind one function boundary (per Decisions, so it can
  become incremental later without touching callers). Deliverable: `allele-worktrees.html`'s table,
  overlap matrix, and merge queue panel all render against real concurrently-open branches. _(PRD
  Phase 1.)_ _(completed 2026-10-02 — **worktree discovery is on-demand ref-sync, not a push-time
  hook, a real decision this phase made rather than inheriting one**: `syncWorktrees` walks the bare
  repo's own `git for-each-ref` output and reconciles the `worktrees` table against it (new rows for
  branches it hasn't seen, `MERGED` for rows whose branch disappeared) every time `ListWorktrees` is
  called — no `CreateWorktree` RPC exists, matching the plan's own "opening one is observed, not
  commanded." A pre-receive hook remains Phase 8's own job specifically (permission *enforcement* at
  push time needs one; simply noticing a branch exists doesn't). Owner identity is a disclosed
  placeholder (`git log`'s own commit-author email, `WorktreeOwnerKind.HUMAN` always) until Phase 8
  replaces it with the real, server-enforced `X-Authentik-Username`.
  <br><br>**`Change` is computed fresh from git on every call, never persisted** — explicitly
  decided, not an oversight: a cached row risks going stale the instant a worktree's branch is
  pushed to again, and recomputing is cheap (it's exactly Phases 5/6's own diff/merge core, already
  built). A `Change`'s own id is simply its worktree's id, since an open worktree has exactly one
  current Change.
  <br><br>**The overlap matrix found and closed a real gap in Phase 6's own `merge.Overlap`**: that
  function only ever tracked symbols *both sides actually touched by name* — two worktrees editing
  completely different symbols in the *same file* (`allele-worktrees.html`'s own most common
  "merges cleanly" case) produced empty `AutoMerged` and empty `Conflicts` alike, indistinguishable
  from no overlap at all. Added `SharedFiles` to `OverlapResult` to close this before building
  `GetOverlapMatrix` on top of it, rather than shipping the gap into the RPC layer and discovering
  it there instead.
  <br><br>**The merge queue's own verification is the AST check only, named plainly as such** — one
  `VerificationStep`, "Merge simulation with open worktrees", `PASSED`/`FAILED` based on
  `merge.Overlap` against every other currently-open worktree. The Bench-run half (real unit/
  contract tests, more `VerificationStep` entries) is explicitly Phase 10's own job; this phase
  doesn't simulate or stub it under a misleadingly complete-looking checks list.
  <br><br>**A readability bug found and fixed from the live output itself, not from reading code**:
  the first version stamped each worktree's own database UUID into `ConflictReport` explanations
  ("9652882c-d987-... removes the field..."), because `worktreeOverlap` passed `a.ID`/`b.ID` into
  `merge.LoadSideSymbols` where the plan's own design intended a human-readable label. Fixed to pass
  the branch name instead — `OverlapPair.worktree_id_a`/`worktree_id_b` still carry the real
  worktree ids (for the UI to link a matrix cell to its worktree); only the prose inside a
  `ConflictReport` changed.
  <br><br>**Live-verified against the real running service, not a CLI simulation**: created a real
  repository over Connect's JSON protocol, pushed three real branches to it with a real `git
  clone`/`push` (the exact `wt-04`/`wt-07` broken-reference scenario, plus a genuinely unrelated
  third branch), and confirmed via the live RPCs: `ListWorktrees` discovered all three from their
  real refs with correct branch/base-commit/owner-email; `GetWorktreeOverlap` found exactly the one
  real `CONFLICT_KIND_SEMANTIC_CONFLICT` pair, explanation reading "wt-04 removes the field
  Upstream.Address that Pick starts reading"; `GetChange` on `wt-07` returned its real modified
  `Pick` symbol, real line-count stats (`git diff --numstat`, not estimated), and the same conflict
  embedded directly in the Change; `EnqueueMerge` correctly `FAILED` both conflicting worktrees
  (setting them `BLOCKED`) and correctly `WAITING`+`PASSED` the unrelated third one; `ListMergeQueue`
  returned all three in correct position order; `HoldMergeQueueEntry`/`ResumeMergeQueueEntry`
  correctly toggled `held`. Also found, while wiring this up, that the watch-managed dev stack's own
  `start_watched` entry for Allele only watched `main.go`/`go.mod`/`go.sum`/`config.yaml` — every
  subpackage this plan has added since Phase 4 (`parsing`, `diff`, `merge`, `gitutil`, `cmd/*`) and
  every new top-level file this phase added would silently not trigger a rebuild on save. Fixed in
  both `run-local.sh` (unaffected — a plain `go build .` already covers the whole module) and
  `run-local-watch.sh` (added one `-w` per top-level file and per subpackage directory). `go build`/
  `go vet`/`gofmt` clean throughout.)_
- [x] **8. Provenance and permission enforcement (R4).** Authentik-identity-driven commit
  attribution; `PathPermission` enforcement inside `pre-receive`; Ed25519 server-side signing of
  accepted merge commits for `AGENT`/`EVOLUTION_RUN` worktrees; `RegisterSigningKey` plus
  `ssh-keygen -Y verify`-based signature verification for `HUMAN` worktrees; `GetProvenance`.
  Deliverable: a push outside an identity's granted paths is rejected; an accepted agent merge's
  commit is independently verifiable (`git log --show-signature`) against Allele's own published key
  for that identity; a human's locally-signed commit verifies against their registered
  `SigningKey`, and an unsigned human commit is still accepted. _(PRD Phase 1.)_ _(completed
  2026-10-02 — scope split explicitly: R4.1 (attribution) and R4.2 (path-permission enforcement) are
  fully built and live-verified below.
  <br><br>**R4.3's signing subsystem is not built**: Ed25519 server-side signing for
  `AGENT`/`EVOLUTION_RUN` worktrees, `RegisterSigningKey`/`ListSigningKeys`/`RevokeSigningKey`, and
  `ssh-keygen -Y verify`-based verification for `HUMAN` worktrees remain `CodeUnimplemented` stubs. A
  cryptographic signing/verification subsystem deserved its own pass rather than a rushed one bolted
  onto this phase; it's the one piece of this phase's stated scope being carried forward undone,
  stated plainly rather than silently dropped.
  <br><br>**The load-bearing environment-propagation assumption was verified before anything was built
  on top of it**: that an HTTP header sent via `git -c http.extraHeader="X-Authentik-Username: ..."`
  actually propagates through `net/http/cgi.Handler` → `git-receive-pack` → a `pre-receive` hook's own
  environment as `HTTP_X_AUTHENTIK_USERNAME`, via plain OS environment inheritance. Confirmed with a
  disposable test repo and a diagnostic hook first; also confirmed git's quarantine mechanism
  (`GIT_QUARANTINE_PATH`/`GIT_OBJECT_DIRECTORY`) is live and inherited at pre-receive time, which is
  what lets the hook's own `git diff` see a push's not-yet-accepted objects.
  <br><br>**`Provenance` is keyed by commit sha, not change id**, despite `GetProvenanceRequest` taking
  a `change_id` — the pre-receive hook runs *before* the ref is updated, so a brand-new branch's very
  first push has no worktree row yet to key a `change_id` against (worktree discovery is Phase 7's
  on-demand `syncWorktrees`, triggered by the next `ListWorktrees` call, not by the push itself).
  `GetProvenance` resolves `change_id` → worktree → its branch's current `HeadCommit` → the provenance
  row for that exact sha, at read time, the same "derive at read time, store nothing extra" discipline
  `computeChange` already established for `Change` itself in Phase 7. `CheckPushPermissionRequest`
  grew two fields beyond what Phase 2 originally specced (`ref`, `new_commit_sha`) once the design made
  clear it needed both: `ref` to resolve an existing worktree's `OwnerKind` for the recorded row,
  `new_commit_sha` as the row's own primary key. A provenance row is written only on an *allowed* push,
  never a denied one — a rejected push enters nothing, and its rejection is already surfaced
  synchronously to the pusher via the hook's own non-zero exit, so R4.1's "queryable after the fact"
  has nothing further to add for it. `evaluatePathPermissions` denies by default when an identity has
  zero `PathPermission` rows at all (not an open door), and a `deny_globs` match always wins over an
  `allow_globs` match on the same path — glob matching (`pathMatchesGlob`, supporting `**` across path
  segments and `*` within one) is a direct glob→regex translation rather than a new dependency, scoped
  to exactly this one job.
  <br><br>**`cmd/pre-receive-hook` is a standalone, dependency-light binary** (stdlib networking plus
  the one generated proto package, for Connect's plain-JSON wire format — the same curl-able shape
  used throughout this plan's own verification, not a generated Connect client) installed into every
  new repository by `store.installHook` as a small generated shell script baking in that repository's
  own UUID and this server's address (`ALLELE_REPOSITORY_ID`/`ALLELE_SERVER_ADDR`), rather than the
  hook needing to re-derive either. `CreateRepository` now fails outright (rolling back the directory)
  if hook installation fails — a repository silently missing its own enforcement hook is a real
  security gap, not a cosmetic one. `main.go` resolves the hook binary as `allele-pre-receive-hook`
  next to its own `os.Executable()`; both `run-local.sh` and `run-local-watch.sh` now build it as
  allele's sibling binary (the watch variant rebuilds it alongside allele itself on every relevant
  save). A repository created before this phase's hook existed is **not retrofitted** — it keeps
  running with no push enforcement until recreated; only `CreateRepository` installs the hook.
  <br><br>**Two real bugs found live, both on the very first real pushes through the hook, not via code
  review**: (1) the hook's own `runGit` helper returned raw, untrimmed `git merge-base` stdout — a
  brand-new branch's first push spliced that trailing newline straight into a `base..compare` diff
  range, producing a single garbled argument and a bogus "ambiguous argument" failure from `git diff`
  instead of a real changed-path list; fixed by trimming at the one call site that needed it
  (`changedPaths`'s own call already trimmed correctly, which is what made this one easy to miss by
  inspection alone). (2) `ownerKindForRef`'s worktree lookup compared `ref` (as git's pre-receive hands
  it to the hook, e.g. `refs/heads/wt-01`) directly against `worktreeRow.Branch` (short names only, per
  `gitutil.ListBranches`'s `%(refname:short)` format) — every lookup silently missed, indistinguishable
  from the legitimate "no worktree yet" case, until a second push to an already-discovered branch
  should have shown a real `OwnerKind` and didn't. Fixed with a `branchNameFromRef` helper stripping
  the prefix. Both bugs were invisible to `go build`/`go vet`/`gofmt` and would have been invisible to
  a unit test against a mocked query — exactly the class of bug this plan's "live-verify against the
  real running service" standard exists to catch.
  <br><br>**Live-verified end-to-end against the real running dev stack**, not a CLI simulation:
  created a real disposable repository (`phase8/permtest`) through the real `CreateRepository` RPC and
  confirmed the hook was actually installed on disk with the right repository id and server address
  baked in; granted `agent-3@example.com` `allow_globs: ["src/**"]`/`deny_globs: ["src/secrets/**"]`
  via `SetPathPermissions`; a real `git clone`/`git push` (with `-c
  http.extraHeader="X-Authentik-Username: ..."`, the same mechanism Authentik's own reverse-proxy would
  set) touching only `src/feature.go` succeeded cleanly; a push touching `src/secrets/key.pem` (inside
  both `allow_globs` and `deny_globs`) was rejected with the correct deny-wins reason; a push touching
  `docs/readme.md` (outside every `allow_globs` entry) was rejected with the correct not-covered
  reason; a push with no `X-Authentik-Username` header at all was rejected via the deny-by-default path
  (`identity "" has no granted path permissions`); a commit whose real git author
  (`someone-else@example.com`) deliberately differed from its push's identity header
  (`agent-3@example.com`) recorded provenance under the *header* identity, not the spoofable
  git-commit-author field, confirming R4.1 actually enforces server-side attribution rather than
  trusting the commit itself. `GetProvenance` returned `OwnerKind` unset (`UNSPECIFIED`) for a
  worktree's first-ever push (no worktree row existed yet to classify against — the disclosed ordering
  gap above) and correctly populated (`WORKTREE_OWNER_KIND_HUMAN`) on the next push to the same branch
  once discovery had caught up. `SetPathPermissions`'s full-replace semantics were confirmed by
  granting a second, unrelated identity and verifying `ListPathPermissions` showed only the new set.
  `GetProvenance` against a nonexistent id returned a clean `CodeNotFound`, not a crash. `go build`/
  `go build ./...`/`go vet`/`gofmt` all clean throughout, including `cmd/pre-receive-hook` as its own
  built artifact, not just type-checked.)_
- [x] **9. Catalyst events, Foundry, and telemetry (§9).** The five CloudEvent types from
  [Telemetry](#telemetry); Foundry manifest publish/retract via `chassis.Effect`; wide events per
  mutating RPC. Deliverable: a standalone `Consume` subscriber receives a real
  `tooling.allele.v1.SymbolChanged` event during a live push; Allele listed in Foundry's catalog
  while running. _(PRD Phase 1.)_ _(completed 2026-10-02 — three of Telemetry's five CloudEvent
  types are wired and live-verified: `SymbolChanged`, `ConflictDetected`, `VerificationCompleted`.
  The other two have no real mechanism anywhere yet to fire them from honestly: `LinkageChanged`
  needs Phase 11's linkage manifest (doesn't exist), and `MergeCompleted` needs something that
  actually executes a queued, verified change's fast-forward into `main` -- which, on inspection,
  **no phase's design has actually built yet**, including this one's own merge queue (Phase 7 only
  enqueues and verifies; nothing dequeues a passed entry and performs the real git-level merge). That
  gap is real and worth flagging plainly rather than inventing a half mechanism just to have
  something to fire an event from.
  <br><br>**`WatchChanges` relabeled from an earlier "Phase 9" placeholder to Phase 12**: its own
  scope was always a direct in-process stream, explicitly *not* a Catalyst `Consume` subscription
  (see the proto's own doc comment) -- it has no real consumer to verify it against until the web
  client exists to open one, so it's deferred there instead of shipped unverified now. Phase 9's own
  checklist text never actually named it; the "Phase 9" string in its original stub was this plan's
  own mislabeling from Phase 2, corrected here.
  <br><br>**`SymbolChanged` needed a new hook, not the existing one**: Phase 8's `pre-receive` hook
  runs *before* a push's objects leave git's quarantine, so only the hook's own process (not Allele's
  long-lived server, reading the real repository on disk) can see them -- diffing from the RPC
  handler at that point would silently see nothing new. A `post-receive` hook (new:
  `cmd/post-receive-hook`, installed by the same `store.installPreReceiveHook`-pattern
  `installPostReceiveHook`) runs *after* objects are committed to the real object store, which is
  what makes server-side diffing safe; it relays each accepted ref update to a new `NotifyPush` RPC,
  which reuses `computeChange` verbatim (Phase 7's "derive at read time, never persist" diff) rather
  than inventing a second diffing path, and fires one `SymbolChanged` event per resulting
  `SymbolChange`. A push straight to the repository's own default branch is a deliberate no-op (not a
  worktree at all). `CheckPushPermissionRequest` and `NotifyPushRequest` both needed small field
  additions (`ref`, `new_commit_sha`) beyond their original Phase 8 shape once the real design made
  clear what each actually needed to carry -- disclosed in Phase 8's own completion note for the
  first, and here for the second.
  <br><br>**The Catalyst publisher (`events.go`) uses an explicit h2c transport, not
  `http.DefaultClient`** -- mirroring `crud-event`'s own event publisher, not `lineman`'s: a bare
  `http.Client` has no h2c support, and `connect.WithGRPC()` needs real HTTP/2 framing to function at
  all against a plain-TCP Catalyst address. `lineman`'s own publisher passes `http.DefaultClient`
  into the identical call shape -- a likely-latent issue in its code, not this plan's to fix, noted
  here only because it directly informed which sibling pattern to copy.
  <br><br>**A real, reproducible shutdown-hang bug was found and fixed, specific to Allele's own new
  code**: the established `eventsCtx`/`cancelEvents` convention (`crud-event`, `lineman`, `relay` all
  use it) reads `chassis.Closer()` directly in a second goroutine -- but `Closer()` returns the one
  package-level, buffer-1 channel chassis's *own* `Start()` reads internally
  (`signal.Notify(closer, ...)` in `runtime.go`) to trigger its graceful `shutdown()` sequence. A
  second reader racing for that same single buffered value can win and drain it first, leaving
  chassis's own internal wait blocked forever -- `shutdown()` (and every effect's teardown, Foundry's
  retract included) then never runs, and the process only dies on `SIGKILL`, not `SIGTERM`.
  Reproduced directly while verifying this phase: a manually-run instance sent `SIGTERM` did not exit
  for 7+ seconds (chassis's own shutdown has a 5-second total budget) until force-killed. Fixed in
  Allele's own `main.go` by registering a real `chassis.Effect` whose teardown cancels `eventsCtx`
  instead -- its dispose is called directly by `shutdown()`'s own loop, no race possible. Verified:
  the fixed binary now exits cleanly (`SIGTERM` to process exit) in 1 second, and a probe publish
  immediately afterward confirmed the Foundry retract teardown had actually run. This is a
  pre-existing, shared pattern across every service listed above, not introduced by Allele -- fixed
  here for Allele's own code only; the others are out of this plan's scope to touch.
  <br><br>**A real, pre-existing Phase 7 schema bug was found live and fixed**: `worktreeRow.Branch`
  carried a bare `bun:"...,unique"` tag -- a *global* unique constraint across every repository this
  instance hosts, not one scoped to a single repository. Invisible in Phases 7-8 purely because no
  test there ever reused a branch name across two different repositories; Phase 9's own live testing
  did exactly that (`wt-01` in two separate test repos) and the second `CreateRepository`-triggered
  worktree discovery failed outright with a real Postgres `23505` unique-violation, surfaced to the
  pusher via the (non-blocking) `post-receive` hook's own stderr. Fixed in the Go model with a
  composite `unique:worktrees_repository_id_branch` tag across `RepositoryID`+`Branch` (the same
  `unique:<name>`-on-both-fields convention `services/tooling/foundry/model.go`'s own
  `plugins_name_version` constraint already established), and in the live table directly (`ALTER
  TABLE ... DROP CONSTRAINT` / `ADD CONSTRAINT`, since worktree rows are fully re-derivable from git
  refs via `syncWorktrees` -- nothing of value would have been lost recreating the table either way,
  but altering in place was simpler and no more risky).
  <br><br>**Tracing spans added for the three Telemetry-named RPCs that exist today**:
  `EnqueueMerge`/`GetWorktreeOverlap`/`SetPathPermissions`, matching the exact
  `chassis.StartSpan`/`SetBusinessAttribute`/`SetRuntimeAttribute`/`span.End(err)` shape
  `relay/service/rpc.go` already established. `ApproveLinkageChange`'s own row in Telemetry's table
  is deferred with the rest of Phase 11 -- the RPC itself doesn't exist yet to instrument.
  <br><br>**Live-verified end-to-end against the real running dev stack**, every event type via a
  real standalone subscriber (the new, kept `cmd/verify-symbol-changed` diagnostic -- parameterized
  to check any of the three event types, not three separate tools for one shared publish path):
  a real `git clone`/`push` of a brand-new repository's worktree branch, through the real installed
  `post-receive` hook, produced a live `tooling.allele.v1.SymbolChanged` event for the pushed
  function, caught by a subscriber started beforehand with no RPC bypass involved; a second,
  deliberately conflicting worktree (`Add` redefined with a different arity) pushed to the same
  repository, then run through a real `EnqueueMerge` call, produced both a live
  `tooling.allele.v1.ConflictDetected` event (the real `SAME_NODE` conflict, correct explanation) and
  a live `tooling.allele.v1.VerificationCompleted` event (`VERIFICATION_STATUS_FAILED`, the same
  step/detail the RPC's own response carried); `allele@v1` confirmed present in a live
  `PluginCatalogService.List` call alongside every other real tooling service. `go build`/`go build
  ./...`/`go vet`/`gofmt` clean throughout, both new hook binaries included; `api`'s own `go
  build`/`cargo check` clean after the proto regen.)_
- [x] **10. Verification loop via Bench (R3).** `.allele/verify.yaml` convention; `EnqueueMerge`
  triggering a real Bench `TriggerRun`, polling `GetRun`, surfacing per-step status and failure
  detail (R3.3) as `VerificationStep`s. Deliverable: a change with a real failing test is rejected
  from the merge queue with the specific failing step and its detail visible, matching
  `allele-change.html`'s own Verification loop panel. _(PRD Phase 2.)_ _(completed 2026-10-02 --
  fully built and live-verified, including a found-and-worked-around gap in Bench's own API.
  <br><br>**Load-bearing finding: Bench's `TriggerRun` never actually reads its own `inputs`
  field**, confirmed by reading `services/tooling/bench/rpc.go`'s handler directly (it never touches
  `req.Msg.GetInputs()`) and `templating.go` (its resolver only recognizes
  `{{ steps.X.result.Y }}` -- no `inputs.` namespace exists at all). This plan's own original Decision
  prose ("with the change's commit SHA and changed-symbol list as trigger inputs") assumed a
  mechanism that, on inspection, doesn't actually work today. Rather than extend Bench's own shared
  templating engine to support it -- a cross-service change outside this plan's scope, and exactly
  the kind of shared-infrastructure edit this project has otherwise avoided all the way through --
  Allele does its own minimal, disclosed substitution directly on the `.allele/verify.yaml` text
  before Bench ever sees it: `{{ allele.commit_sha }}`, `{{ allele.branch }}`, and
  `{{ allele.repository_name }}` are replaced with concrete values first, so by the time Bench
  registers and runs the workflow it's just an ordinary, fully-resolved one with no placeholders of
  any kind left for Bench's own templating to need to understand.
  <br><br>**Each worktree gets its own Bench workflow, named `allele-<worktree-id>`, never the
  committed file's own `metadata.name`** -- forced via a targeted rewrite of just that one YAML node
  (walking the real `yaml.Node` tree, the same low-level approach
  `services/tooling/bench/validate.go`'s own diagnostics are built on; a blind text/regex replace was
  considered and rejected since a `Step`'s own `name:` field uses the identical YAML key one level
  down, and this plan has no serialization/locking around concurrent `EnqueueMerge` calls in the same
  repository that would make a single shared, repo-level workflow name safe from one worktree's
  registration racing another's). `UpdateWorkflow` is tried first (the common case: iterative work
  re-enqueueing the same worktree), falling back to `CreateWorkflow` on `CodeNotFound` for a
  worktree's very first verification.
  <br><br>**Triggering and waiting are deliberately split across a synchronous half and a background
  one** -- matching this plan's own, already-written "both asynchronous" decision in [Agent worktrees
  are branches](#agent-worktrees-are-branches-and-verification-is-queue-gated-not-push-gated), not a
  new design. `EnqueueMerge` itself only registers the workflow and calls `TriggerRun`, returning
  immediately with the entry in `VERIFICATION_STATUS_RUNNING` and the worktree in the (previously
  unused) `WORKTREE_STATUS_VERIFYING`; a `pollBenchRun` goroutine (context: `store.bgCtx`, a real
  process-lifetime context cancelled via a `chassis.Effect`, the same correct pattern Phase 9's own
  found shutdown-race fix established -- not a second `chassis.Closer()` reader) calls `GetRun` every
  2 seconds (5-minute cap) until a terminal status, then replaces the placeholder "Bench verification"
  step with Bench's own real, per-step `StepResult`s, updates the worktree's status, and fires
  `VerificationCompleted` -- the event call the synchronous half deliberately skips for `RUNNING`.
  <br><br>**A repository with no `.allele/verify.yaml` now genuinely reaches `PASSED`, not an
  indefinite `WAITING`** -- a real, disclosed behavior refinement to every repository this plan has
  already touched (Phases 7-9's own test repositories included), not a silent one: before Phase 10,
  the AST check alone never had a true terminal success state to report, since "more might come" was
  always at least theoretically true; now that Phase 10 gives real meaning to that "more," its actual
  absence is honestly reportable as done, not forever-pending.
  <br><br>**Startup recovery for in-flight Bench runs**: a merge queue entry can be left
  `Status=RUNNING` with a real `BenchRunID` if the process managing its `pollBenchRun` goroutine dies
  mid-verification -- a routine occurrence in this dev stack's watch-mode restarts, not a rare crash
  worth ignoring. `recoverPendingBenchRuns`, called once at startup, finds exactly these rows and
  relaunches a poller for each; live-verified by enqueueing a real 15-second Bench run, restarting
  Allele 3 seconds in (killing the original goroutine outright), and confirming the entry still
  correctly reached `PASSED` with the real step result once the new process's recovered poller caught
  up with Bench's own, unaffected, still-running run.
  <br><br>**Live-verified end-to-end against the real running dev stack**, both outcomes, using
  Bench's own built-in `bench://delay@v1` executor (no external test infrastructure needed for a
  clean, fast, reliable check): a worktree committing a passing `verify.yaml` reached
  `VERIFICATION_STATUS_PASSED` with the real step name (`quick-delay-check`) in its final `Checks`,
  and the registered Bench workflow's own description confirmed the placeholder substitution had
  actually run (a real commit sha and branch name, not the literal `{{ allele.commit_sha }}` text); a
  worktree committing a deliberately invalid one (`duration: "not-a-real-duration"`) reached
  `VERIFICATION_STATUS_FAILED` with the specific failing step (`broken-delay-check`) and Bench's own
  real error text (`time: invalid duration "not-a-real-duration"`) visible in `Checks` -- exactly
  R3.3 and this phase's own stated deliverable. `go build`/`go build ./...`/`go vet`/`gofmt` clean
  throughout.)_
- [x] **11. Linkage manifest (§7, R5.1–R5.4).** `draft.linkage.yaml` structural diffing via
  `tree-sitter-yaml`, the typed read-model, `GetLinkageManifest`/`DiffLinkageManifest`, the
  per-repository `BreakingChangePolicy` setting (defaulting to `INTERFACE_ONLY` per Decisions) and
  `ApproveLinkageChange` flow. Deliverable: `allele-linkage.html`'s graph, version table, and
  manifest diff all render against a real branch that changes a link's mode. _(PRD Phase 3, linkage
  half.)_ _(completed 2026-10-02 -- all four RPCs real and live-verified; one deliberate, disclosed
  deviation from this phase's own original prose, and one disclosed integration gap.
  <br><br>**Parsed with plain `yaml.v3`, not `tree-sitter-yaml`** -- a real, considered departure from
  this section's own original wording, made the same way Phase 10 departed from its own "trigger
  inputs" assumption once the actual tradeoff was visible. The stated reason to use a real parser
  here -- "a key reorder or whitespace change never shows up as a change" -- holds exactly as well
  comparing fully-parsed, already-structured `Component`/`Link` Go values (order-independent by
  construction; see `linkage.Diff`) as it would comparing tree-sitter nodes, and
  `draft.linkage.yaml` has one small, fixed, known schema -- the identical shape
  `services/tooling/bench/loader.go` already parses the same way (plain YAML into a typed struct) for
  its own fixed-schema workflow YAML, not the open-ended "any valid program" problem tree-sitter
  earns its keep solving for Go/Rust/.proto elsewhere in this plan. Adding a new grammar dependency
  here would have been complexity without a correctness benefit -- confirmed live, not just argued:
  a branch that only reordered `draft.linkage.yaml`'s own keys and components produced an empty
  `DiffLinkageManifest` response, the exact property the original tree-sitter proposal was for.
  <br><br>**`LinkageManifestChange.component` names the link's own target, not a "source -> target"
  pair** -- matching the proto's own documented example verbatim (`allele-linkage.html`'s
  "beacon-exporter: static -> sidecar" row) rather than inventing a different shape; live-verified
  output matched this exactly, field for field. A component added or removed outright is reported as
  every one of its own links changing, for the same reason: there is no separate "component itself
  appeared/disappeared" signal in this message shape, only link-level changes, so that's the
  information actually carried.
  <br><br>**`BreakingChangePolicy.ALWAYS` and `.INTERFACE_ONLY` are honestly identical today, not an
  oversight**: `LinkageManifestChange` itself has no field for a non-interface difference (e.g. a
  component's own version bump with no link change isn't something `Diff` can even represent, since
  there's no `version_before`/`version_after` on the message) -- every change this phase's `Diff` can
  possibly produce already *is* an interface-level link change, so there is currently no
  non-interface case for `ALWAYS` to be stricter about. The policy is still stored and threaded
  through correctly end-to-end (confirmed live: `SetBreakingChangePolicy` persisted and was
  immediately reflected in a follow-up `GetLinkageManifest`); the two policies only start behaving
  differently once a future phase adds a representable non-interface change.
  <br><br>**`ApproveLinkageChange` is real and persisted, but not yet consulted by `EnqueueMerge`** --
  a disclosed integration gap, not a silent one. `change_id` reuses the existing worktree-id
  convention (`ApproveLinkageChangeRequest` has no other identifier to mean), and a real
  `linkage_approvals` row is written and confirmed queryable, but nothing in the merge queue's own
  pass/fail logic yet calls `GetLinkageManifest`/`DiffLinkageManifest`/`IsBreaking` against the stored
  policy, or checks for an approval, before deciding a `Change`'s status. "A breaking change blocks
  the queue until approved" (R5.4's own underlying intent) is a natural next increment, explicitly
  not claimed done here.
  <br><br>**Live-verified end-to-end against the real running dev stack**: `GetLinkageManifest`
  against a repository with no `draft.linkage.yaml` yet returned a real commit sha, zero components,
  and the correct default policy (`BREAKING_CHANGE_POLICY_INTERFACE_ONLY`) rather than an error;
  after committing a real manifest and a worktree that moved one link from `static` to `sidecar`,
  `DiffLinkageManifest` returned exactly that one change with the right before/after modes; a
  reorder-only worktree (components and keys both shuffled, no real edit) against the same base
  produced an empty diff, confirmed above; `SetBreakingChangePolicy` to `ALWAYS` persisted correctly
  and was reflected immediately in a follow-up read; `ApproveLinkageChange` wrote a real, confirmed
  row. `go build`/`go build ./...`/`go vet`/`gofmt` clean throughout.)_
- [ ] **12. Web client.** `services/tooling/allele/web-client`, Dioxus, implementing
  `allele-repos.html`, `allele-change.html`, `allele-worktrees.html`, and `allele-linkage.html`
  against the real RPCs from Phases 3–11 (built incrementally alongside each phase in practice,
  listed once here for completeness, the same framing Relay's own Phase 10 used). The Evolution nav
  entry is omitted entirely until the evolution plugin exists, rather than shipped as a dead link —
  the same "don't build ahead of need" discipline [already applied
  elsewhere](/docs/architecture/bench-workflow-engine#open-questions) in this doc set. Deliverable:
  all four pages render against real data, no mock/simulated diff, matrix, or manifest data left in
  the shipped client.
- [ ] **13. LSP-based semantic conflict detection (R2.5).** `gopls`/`rust-analyzer` integration for
  conflict detection beyond syntax (type compatibility, cross-file reference breakage) — explicitly
  a later phase per the PRD itself (§8.3, §12 Phase 3), included here only for traceability, not
  detailed further since the PRD itself treats it as "not a blocker" to everything above.
- [ ] **14. Verification.** Live, end-to-end: clone and push against a real Allele-hosted repo with
  no AST involvement (Phase 3's own claim, re-checked once later phases exist); two real concurrent
  branches with an unrelated same-line change auto-merge and a real cross-file broken reference is
  caught and correctly explained; a path-permission violation is rejected; an accepted merge's
  commit signature verifies independently; a failing Bench verification blocks a merge-queue entry
  with real failure detail; a `draft.linkage.yaml` mode change renders correctly and respects its
  repository's breaking-change policy; a standalone Catalyst subscriber observes all five event
  types live.

## Open Questions

Resolved during review 2026-10-01 and folded into [Decisions](#decisions): the
`.allele/verify.yaml`-reads-from-the-target-branch rule and its per-path (CODEOWNERS-style)
scoping, the `BreakingChangePolicy` default, the overlap matrix's O(n²)-now/isolated-for-later
design posture, Allele owning its own signing-key table independent of `agents.md`, humans holding
their own SSH key rather than being server-signed, and the verification-sandbox split between
Allele's own in-process AST check and Bench's existing execution isolation. What's left:

- **Verification loop structure (R3.2), one residual detail.** The exact `TriggerRun` input shape —
  which variables a `.allele/verify.yaml` author can reference (commit SHA, changed-files list,
  symbol list) — needs a first real repository's verification needs to design against, rather than
  being guessed here.
- **Concurrency targets (R1.3)** — still no specific numeric target. Confirmed with the user as
  intentional: design for growth rather than commit to a number the PRD itself hasn't set. Matters
  most for the overlap matrix (see [Decisions](#decisions) for the isolated-O(n²) posture that keeps
  this revisitable) and for local-disk sizing under [the single-instance decision](#decisions).
- **Latency targets** — still fully open per the PRD; needs real usage data before committing to a
  number for either the AST conflict check or Bench-backed verification. Not blocking: the
  queue-gated design in [Agent worktrees are
  branches](#agent-worktrees-are-branches-and-verification-is-queue-gated-not-push-gated) already
  keeps push latency independent of whatever this number turns out to be.

## Milestone Summary

| Phase | Scope | Status |
|---|---|---|
| 1 | Scaffolding | Done |
| 2 | Proto: `models.proto`, `service.proto` | Done |
| 3 | Git hosting (no diffing) | Done |
| 4 | Tree-sitter parsing integration | Done |
| 5 | Two-way structural diff + `SymbolChange` | Done |
| 6 | Three-way merge + conflict detection (R2.1–R2.3) | Done |
| 7 | Worktrees, overlap matrix, merge queue (R1) | Done |
| 8 | Provenance and permission enforcement (R4) | Done (R4.1/R4.2; R4.3 signing deferred) |
| 9 | Catalyst events, Foundry, telemetry | Done (3/5 events; LinkageChanged/MergeCompleted deferred) |
| 10 | Verification loop via Bench (R3) | Done |
| 11 | Linkage manifest (§7, R5.1–R5.4) | Done (EnqueueMerge gating not yet wired) |
| 12 | Web client (all four in-scope mockup pages) | Not started |
| 13 | LSP-based semantic conflict detection (R2.5) | Not started |
| 14 | Verification | Not started |
