# Allele — Agentic Git Server

**Product Requirements Document**

| | |
|---|---|
| **Status** | Early exploration — concepts and positioning settled, implementation not started |
| **Component** | `Allele` (agentic git server), part of the Draft platform |
| **Owner** | Andrew / steady-bytes |
| **Last updated** | 2026-10-01 |
| **Repo** | [github.com/steady-bytes/draft](https://github.com/steady-bytes/draft) |

---

## 1. Summary

Allele is a purpose-built git server for agentic development. Its core
differentiator is AST-level diffing: conflicts are found and resolved
by comparing what changed in a file's syntax tree, not which lines
moved. This is the precondition for the thing Draft actually needs —
many agents committing to the same codebase in parallel, continuously,
without a human untangling every merge by hand.

Allele is not a replacement for git, and it is not a GitOps deployment
tool. It is a git server with a different diff and merge engine at its
core, built around four concepts — parallel agent worktrees, semantic
conflict detection, structured verification loops, and provenance and
permission enforcement — plus a data model (the **linkage manifest**)
for tracking how components are composed, versioned the same way code
is.

*(Naming note: Allele — a variant form of a gene — fits Draft's
existing naming convention of biological/process metaphors (Catalyst,
Blueprint, Fuse). It is a coincidental but fitting echo of the
separate genetic-algorithm/evolution-plugin research also underway in
Draft; the two are independent components and this PRD does not
assume any dependency between them.)*

---

## 2. Background & Problem Statement

Git's diff and merge model is line-based: it compares text, not
meaning. Two failure modes follow directly from that, and both get
worse as the number of agents committing in parallel increases:

- **False conflicts.** Two agents can touch the same line for
  completely unrelated reasons — one renaming a parameter, one
  reordering an import on an adjacent line — and git flags it as a
  conflict requiring manual resolution, even though nothing about the
  code actually clashes.
- **Missed conflicts.** Two changes that don't share a single line can
  still break each other's assumptions — a function signature changed
  in one file, a caller relying on the old signature changed
  elsewhere — and text diff has no way to see it, because it never
  looks at what either change actually means.

A single human developer tolerates this because the volume of
concurrent change is low enough to resolve by hand. Draft's premise —
a swarm of agents operating across idea, discovery, design,
implementation, and observation — breaks that assumption. The
implementation phase specifically depends on many agents proposing,
testing, and merging changes continuously; a git layer that produces
constant false-positive conflicts, or silently accepts real ones,
becomes the bottleneck the rest of the system is built to avoid.

---

## 3. Goals

- Replace line-based conflict detection with AST-level structural
  comparison, for the languages Draft is actually built in first.
- Support many agents working against the same repository
  concurrently, each in an isolated worktree, without serializing on a
  shared lock.
- Make merge a verified step, not just a textual resolution — a merge
  that resolves cleanly should still be checked before it's accepted
  into shared history.
- Record who (which agent, under what authority) made every change, as
  a first-class, server-enforced property — not a convention that can
  be skipped.
- Track component composition (static link, runtime service, sidecar)
  as its own versioned, branchable, diffable graph — the **linkage
  manifest** — using the same underlying diff/merge machinery as code.
- Emit structured, AST-level change events that downstream Draft
  components (Catalyst, the Planner, the OODA loop) can act on
  directly, without re-parsing a text diff themselves.

## 4. Non-Goals

- **Allele is not a GitOps deployment tool.** It does not decide what
  gets deployed, when, or how. That decision stays downstream,
  informed by the events Allele emits. Allele's responsibility ends at
  producing a trustworthy, structured signal about what changed and
  whether it's safe to merge.
- **Allele is not a general-purpose replacement for git as a
  protocol.** It is a git server implementation with a different
  internal diff/merge/conflict engine; it is not proposing a new
  version-control protocol or asking Draft's components to speak
  anything other than git on the client side.
- **Allele does not perform full program verification.** Structured
  verification loops (§6.3) check specific, defined properties before
  a merge is accepted — they are not a claim of formal correctness
  over arbitrary code changes.
- **Allele is not scoped to support every language on day one.** See
  §8 for the explicit phasing on language support.

---

## 5. Users & Use Cases

| User | Need |
|---|---|
| An agent proposing a code change | Commit into its own worktree without blocking or being blocked by other agents working elsewhere in the same repo. |
| An agent reviewing/merging changes | Get a real conflict signal — not a false positive from an unrelated rename, not a false negative on a change that silently breaks a caller. |
| A human reviewing agent-authored history | See who made a change, under what authority, and why a merge was accepted or flagged — without reconstructing it from commit messages. |
| Draft's Planner / OODA loop | Consume structured, AST-level change events directly, to make deployment and sequencing decisions without re-deriving them from raw diffs. |
| A future structural-search run (the evolution plugin) | Use the linkage manifest as a versioned, diffable representation of system composition — a candidate's structural genome, in the terms used by that separate effort. |

---

## 6. Functional Requirements

### 6.1 Parallel Agent Worktrees

Multiple agents must be able to work against the same repository
simultaneously, each in its own isolated worktree, without waiting on
a shared lock held by another agent's in-progress work.

- **R1.1** — Creating a new agent worktree must not block or be
  blocked by any other worktree's operations against the same
  repository.
- **R1.2** — Each worktree is isolated: an agent's uncommitted or
  unmerged state is not visible to other agents until it is actually
  merged into shared history.
- **R1.3** — The number of concurrent worktrees a repository supports
  must scale with the number of agents Draft expects to run
  concurrently against one codebase (exact target volume: open
  question, §13).

### 6.2 Semantic Conflict Detection (AST-level)

Conflict detection must operate on each changed file's syntax tree,
not its text.

- **R2.1** — Two changes to the same file that touch different,
  non-overlapping AST nodes must never be flagged as conflicting,
  regardless of line-level overlap.
- **R2.2** — Two changes to the same AST node (e.g. both renaming the
  same parameter) must be evaluated structurally: if the edits are
  disjoint in what they change about that node (e.g. one changes the
  name, the other the type), they should auto-merge; if they
  genuinely clash, they are flagged — not silently picked one way.
- **R2.3** — The system must surface *why* something was flagged as a
  conflict or resolved automatically, in terms of what changed
  structurally — not just "lines 12–14 conflict."
- **R2.4** — Language support is explicitly phased; see §8 for the
  parsing and diffing approach and which languages ship first.
- **R2.5** (stretch, phase 2+) — Where feasible, conflict detection
  should extend beyond syntax to semantics a language server already
  understands (type compatibility, cross-file reference breakage) —
  see §8's LSP-integration discussion. This is explicitly out of scope
  for the initial implementation.

### 6.3 Structured Verification Loops

A merge that resolves cleanly at the AST level is not automatically
accepted — it passes through a verification step first.

- **R3.1** — Every merge into shared history passes through a defined
  verification step before being accepted, not just a textual/
  structural resolution.
- **R3.2** — What "verification" checks (tests, contract compliance,
  something else) must be configurable per-repository or per-path, not
  hard-coded into the server. (The exact verification model — what
  runs, who defines it, how failures are surfaced — is an open
  question; see §13.)
- **R3.3** — A merge that fails verification must be rejected (or
  flagged for review) with enough structural + verification detail
  that an agent or human can act on the failure without re-deriving
  what happened.

### 6.4 Provenance & Permission Enforcement

Every change carries an enforced record of who made it and under what
authority.

- **R4.1** — Every commit is attributed to a specific agent identity
  (not just a generic service account), enforced by the server, not
  self-reported by the client.
- **R4.2** — Permission to commit, merge, or modify specific paths is
  enforced at the server based on that identity — an agent cannot
  write outside what it's authorized for by simply choosing to.
- **R4.3** — Provenance information (who, when, under what authority,
  against what verification result) must be queryable after the fact,
  not just logged at commit time and discarded.

---

## 7. The Linkage Manifest

Component composition — whether one component is statically linked
into another, called as a runtime service, or run as a sidecar — is
modeled as its own graph, independent of (but versioned alongside)
source code.

![Linkage manifest graph showing Executor, Catalyst, Blueprint, and a sidecar connected by static-link, runtime-service, and sidecar composition modes](images/allele-linkage-manifest.png)

- **R5.1** — The linkage manifest is version-controlled using the same
  underlying mechanism as source: it has history, it can be branched,
  and changes to it can be diffed.
- **R5.2** — Each edge in the manifest graph is typed by composition
  mode (static link / runtime service / sidecar, at minimum — the set
  of modes may grow).
- **R5.3** — A diff against the linkage manifest must show
  *structural* changes (an edge's composition mode changed, a node was
  added or removed) rather than a raw text diff of whatever file
  format backs it.
- **R5.4** — How the manifest's version history interacts with a
  *breaking* change in composition mode (e.g. a runtime service
  becoming a sidecar) is an open question — see §13.

---

## 8. Technical Approach — Parsing & Diffing

This section reflects the approach discussed and agreed on for how AST
support is actually built, rather than requiring a bespoke parser and
diff algorithm per language from scratch.

### 8.1 Parsing — reused, not rebuilt, per language

Parsing is handled by **tree-sitter**, an incremental parsing library
with community-maintained grammars already published for every
language in Draft's current stack (Go, Rust, TypeScript, Python, SQL,
and `.proto` itself). The work per language is *integrating* an
existing grammar, not writing a parser. [`difftastic`](https://github.com/Wilfred/difftastic)
is a useful existing reference implementation of exactly this pattern
— structural diffing across 30+ languages via per-language tree-sitter
grammars feeding one diff engine.

### 8.2 Structural diff/merge — mostly language-agnostic

Once a file is parsed into a tree-sitter tree, the actual diff
algorithm — matching nodes between two trees, computing tree edit
distance, classifying an edit as a move vs. a modification vs. a real
conflict — is written **once**, against tree-sitter's generic tree
shape, and is not rewritten per language. This is the approach taken
by [GumTree](https://github.com/GumTreeDiff/gumtree) and is the model
Allele's diff engine should follow: one structural-diff core, N
grammars feeding it.

### 8.3 Where per-language work reappears: semantics, not syntax

Tree-sitter gives syntax structure — it knows a token is an
identifier, not whether that identifier is a function parameter, a
type, or a local variable, and it has no notion of whether a change in
one file breaks an assumption in another. The false-conflict
elimination case (R2.1–R2.2) is a syntactic-level call and tree-sitter
alone is sufficient for it. Deeper semantic conflict detection (type
compatibility, scope resolution, cross-file reference tracking — R2.5)
needs language-aware tooling, and the pragmatic path there is
integrating each language's existing **Language Server Protocol**
implementation (`gopls` for Go, `rust-analyzer` for Rust) rather than
building bespoke type-checkers.

### 8.4 Initial language scope

Draft's actual current surface area is narrow: Go, Rust, and
`.proto`. The initial implementation should cover exactly these three
grammars plus the generic tree-diff layer — this is sufficient to
deliver the highest-value, most common case (false-conflict
elimination, R2.1) without requiring LSP-based semantic analysis
(R2.5) to ship anything at all. LSP integration is treated as a
distinct, later phase (§9, Phase 2), not a blocker.

---

## 9. System Integration & Scope Boundary

![Scope boundary: Allele emits structured change events downstream; it is explicitly not a GitOps deployment tool](images/allele-scope-boundary.png)

Allele's output is a structured, AST-level change event — not a
deployment decision. Consumers of that event include:

- **Catalyst** — Draft's event bus; change events are recorded the
  same way any other CloudEvent is.
- **The Planner / OODA loop** — consumes structured change data to
  make sequencing and deployment decisions, informed by what actually
  changed structurally rather than a re-parsed text diff.

Allele's responsibility ends at producing that signal. What happens
with it — whether something gets deployed, re-tested, or escalated to
a human — is explicitly downstream and out of scope for this
component.

---

## 10. Non-Functional Requirements

- **Performance** — conflict detection and merge verification must
  complete fast enough not to become the bottleneck in an
  agent-parallel workflow; exact latency targets are an open question
  pending real usage data (§13).
- **Auditability** — provenance data (§6.4) must be retained and
  queryable, not just logged transiently.
- **Extensibility** — adding a new language should mean integrating a
  tree-sitter grammar against the existing generic diff core, not
  extending the core diff algorithm itself.
- **Security** — permission enforcement (R4.2) must be enforced
  server-side; client-reported identity or authority must never be
  trusted on its own.

---

## 11. Design Reference

The visuals below are from the current design pass for Allele's
public-facing page, included here because they illustrate the model
this PRD describes more directly than prose alone.

**Hero — the core loop**: parallel agents committing into a shared
history, gated by an AST-level conflict check that either confirms no
real conflict or flags for review.

![Allele hero section — three agents committing in parallel, gated by an AST-level conflict check](images/allele-hero.png)

**The differentiator — text diff vs. AST diff on the same edit**: the
concrete case from R2.2 — two agents renaming the same parameter for
unrelated reasons, shown as a false conflict under text diff and a
correct auto-merge under AST diff.

![Side-by-side comparison of a text-diff false conflict and the equivalent AST-diff resolution](images/allele-diff-comparison.png)

**The four core concepts**, as described to prospective users of the
system — corresponds directly to §6 of this document.

![Four concept cards: parallel agent worktrees, semantic conflict detection, structured verification loops, provenance and permission enforcement](images/allele-core-concepts.png)

---

## 12. Phasing

| Phase | Scope |
|---|---|
| **Phase 0** | Tree-sitter integration for Go, Rust, `.proto`. Generic structural diff core (GumTree-style). False-conflict elimination (R2.1, R2.2) working end to end. |
| **Phase 1** | Parallel worktree support at the server level (R1.1–R1.3). Provenance and permission enforcement (R4.1–R4.3). Structured change events emitted to Catalyst (§9). |
| **Phase 2** | Structured verification loops (R3.1–R3.3) — definition of what "verification" means per-repository, and the rejection/flagging flow. |
| **Phase 3** | LSP integration for deeper semantic conflict detection (R2.5). Linkage manifest versioning (§7) fully integrated with the same diff core. |

This phasing is a proposed sequencing based on what each phase depends
on technically (Phase 0's diff core is a prerequisite for everything
after it), not a committed schedule.

---

## 13. Open Questions

- **Verification loop structure** (R3.2) — what exactly runs during
  verification, who defines it per-repository, and how a failure is
  surfaced to the agent or human who needs to act on it.
- **Linkage manifest breaking changes** (R5.4) — how version history
  should represent a composition-mode change (e.g. runtime service →
  sidecar) that isn't a simple additive diff.
- **Concurrency targets** (R1.3) — how many simultaneous agent
  worktrees per repository Allele needs to support; no target volume
  has been set yet.
- **Latency targets** — no specific performance budget has been set
  for conflict detection or verification; needs real usage data before
  committing to a number.
- **Persistence model** — whether linkage manifest state and ancestry
  should live in Draft's existing PostgreSQL snapshot mechanism or
  Catalyst's ClickHouse event store (this question was raised in the
  context of the separate evolution-plugin work and may apply here
  too, but has not been resolved for Allele specifically).

## 14. Risks

- **False sense of safety from structural merge.** An AST-level merge
  that resolves cleanly is not a guarantee of correctness — R3 exists
  specifically to avoid treating "merges without conflict" as
  sufficient on its own.
- **LSP integration cost and reliability** (Phase 3) — language
  servers are built for editor use, not as embedded libraries in a git
  server's merge path; integration cost and performance under this
  usage pattern is unproven.
- **Scope creep toward a deployment tool.** §4 and §9 exist explicitly
  to keep Allele's responsibility bounded to producing a trustworthy
  change signal — repeated reinforcement in design review will likely
  be needed as the system grows.

---

## References

- GumTree — fine-grained source code differencing: <https://github.com/GumTreeDiff/gumtree>
- tree-sitter — incremental parsing library: <https://tree-sitter.github.io/tree-sitter/>
- difftastic — structural diff tool across 30+ languages: <https://github.com/Wilfred/difftastic>
- Draft repository: <https://github.com/steady-bytes/draft>
