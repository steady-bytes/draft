// verification.go is Phase 10's own piece: triggering a real Bench-backed verification run from
// EnqueueMerge, instead of the AST-only check Phase 7 shipped alone. See the plan's "Structured
// Verification Loops (R3)" section.
//
// A load-bearing finding from building this: Bench's own TriggerRun handler never reads
// TriggerRunRequest.Inputs at all (confirmed by reading services/tooling/bench/rpc.go directly), and
// its templating engine (templating.go) only recognizes "{{ steps.X.result.Y }}" -- there is no
// "{{ inputs.X }}" (or any other) mechanism for a triggered run to know which commit it's actually
// verifying. Rather than extend Bench's own shared templating engine (out of this plan's scope, and
// exactly the kind of cross-service change this project has otherwise avoided), Allele does its own
// minimal, disclosed placeholder substitution directly on the .allele/verify.yaml text before ever
// handing it to Bench -- by the time Bench sees it, it's just an ordinary, fully-concrete workflow
// with no unresolved placeholders of any kind.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	workflowv1connect "github.com/steady-bytes/draft/api/tooling/workflow/v1/v1connect"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

// verifyYAMLPath is the file a repository commits to opt into Bench-backed verification -- its
// presence (checked once per EnqueueMerge, via gitutil.ShowFile against the worktree's own branch)
// is what makes this step entirely opt-in per repository, matching R3.2's "configurable per-
// repository... not hard-coded into the server."
const verifyYAMLPath = ".allele/verify.yaml"

// substitutePlaceholders replaces Allele's own small, fixed set of tokens with concrete values --
// deliberately NOT a general templating language (no loops, conditionals, or arbitrary expressions):
// just enough for a workflow step to know which commit/branch/repository it's verifying. Documented
// here, not just in the plan doc, since this is the one place a verify.yaml author needs to know
// about them.
func substitutePlaceholders(yamlContent []byte, commitSHA, repositoryName, branch string) []byte {
	s := string(yamlContent)
	s = strings.ReplaceAll(s, "{{ allele.commit_sha }}", commitSHA)
	s = strings.ReplaceAll(s, "{{ allele.repository_name }}", repositoryName)
	s = strings.ReplaceAll(s, "{{ allele.branch }}", branch)
	return []byte(s)
}

// rewriteWorkflowName overwrites just the metadata.name field's value, preserving everything else
// in the document (comments, step order, formatting) -- a per-worktree-unique name is forced
// regardless of whatever the committed file's own metadata.name says, so two different worktrees (or
// two different repositories) that both happen to name their own verify.yaml workflow the same thing
// (e.g. "verify") never collide in Bench's own, globally-named workflow catalog. A plain string/regex
// replace was considered and rejected: a Step's own "name" field uses the identical YAML key at a
// different nesting level, and a blind text replace risks rewriting the wrong one. Walking the real
// yaml.Node tree (the same low-level approach services/tooling/bench/validate.go's own line/column
// diagnostics are built on) finds metadata.name specifically, nothing else.
func rewriteWorkflowName(yamlContent []byte, newName string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(yamlContent, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", verifyYAMLPath, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected a top-level mapping", verifyYAMLPath)
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "metadata" {
			continue
		}
		metadata := root.Content[i+1]
		if metadata.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s: metadata is not a mapping", verifyYAMLPath)
		}
		for j := 0; j+1 < len(metadata.Content); j += 2 {
			if metadata.Content[j].Value == "name" {
				metadata.Content[j+1].Value = newName
				var out bytes.Buffer
				enc := yaml.NewEncoder(&out)
				enc.SetIndent(2)
				if err := enc.Encode(&doc); err != nil {
					return nil, fmt.Errorf("re-encoding %s: %w", verifyYAMLPath, err)
				}
				return out.Bytes(), nil
			}
		}
		return nil, fmt.Errorf("%s: metadata.name is required", verifyYAMLPath)
	}
	return nil, fmt.Errorf("%s: metadata block is required", verifyYAMLPath)
}

// benchStepResultToVerificationStep maps one Bench StepResult onto an Allele VerificationStep --
// this is what gives a merge queue entry the per-step granularity R3.3 ("the specific failing step
// and its detail visible") actually asks for, instead of one opaque "Bench verification" line.
func benchStepResultToVerificationStep(sr *workflowv1.StepResult) *allelev1.VerificationStep {
	detail := sr.GetError()
	if detail == "" {
		// A passed or skipped step has no error, but detail/result may still carry something worth
		// showing (e.g. bench://grpc-call@v1's own resolved target/URL/status, per StepResult's own
		// doc comment) -- best-effort, not required.
		if d := sr.GetDetail(); d != nil {
			if b, err := protojson.Marshal(d); err == nil {
				detail = string(b)
			}
		}
	}
	return &allelev1.VerificationStep{
		Name:   sr.GetStepName(),
		Status: benchStepStatusToAllele(sr.GetStatus()),
		Detail: detail,
	}
}

// benchStepStatusToAllele maps Bench's own StepStatus onto Allele's VerificationStatus -- the two
// enums are intentionally close (Bench's SKIPPED has no Allele equivalent; folded into PASSED, since
// a skipped step didn't fail and shouldn't block a merge).
func benchStepStatusToAllele(s workflowv1.StepStatus) allelev1.VerificationStatus {
	switch s {
	case workflowv1.StepStatus_STEP_STATUS_PASSED, workflowv1.StepStatus_STEP_STATUS_SKIPPED:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_PASSED
	case workflowv1.StepStatus_STEP_STATUS_FAILED:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED
	case workflowv1.StepStatus_STEP_STATUS_RUNNING:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_RUNNING
	case workflowv1.StepStatus_STEP_STATUS_PENDING:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_WAITING
	default:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_UNSPECIFIED
	}
}

func benchRunStatusToAllele(s workflowv1.RunStatus) allelev1.VerificationStatus {
	switch s {
	case workflowv1.RunStatus_RUN_STATUS_PASSED:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_PASSED
	case workflowv1.RunStatus_RUN_STATUS_FAILED:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_FAILED
	case workflowv1.RunStatus_RUN_STATUS_RUNNING:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_RUNNING
	case workflowv1.RunStatus_RUN_STATUS_PENDING:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_WAITING
	default:
		return allelev1.VerificationStatus_VERIFICATION_STATUS_UNSPECIFIED
	}
}

func isTerminalRunStatus(s workflowv1.RunStatus) bool {
	return s == workflowv1.RunStatus_RUN_STATUS_PASSED || s == workflowv1.RunStatus_RUN_STATUS_FAILED
}

// benchClient wraps WorkflowServiceClient with the two higher-level operations EnqueueMerge/the
// background poller actually need -- a plain Connect unary client (http.DefaultClient, no h2c)
// since TriggerRun/GetRun/UpdateWorkflow/CreateWorkflow are all ordinary unary RPCs, unlike
// Catalyst's own streaming Produce/Consume.
type benchClient struct {
	client workflowv1connect.WorkflowServiceClient
}

func newBenchClient(addr string) *benchClient {
	return &benchClient{client: workflowv1connect.NewWorkflowServiceClient(http.DefaultClient, addr)}
}

// registerWorkflow makes name's definition in Bench match yamlContent exactly, creating it if this
// is the first time this worktree has ever enqueued a merge, updating it otherwise -- see
// rewriteWorkflowName's own doc comment on why name is always worktree-specific, never the file's
// own metadata.name.
func (b *benchClient) registerWorkflow(ctx context.Context, name string, yamlContent []byte) error {
	_, err := b.client.UpdateWorkflow(ctx, connect.NewRequest(&workflowv1.UpdateWorkflowRequest{
		Name: name,
		Yaml: string(yamlContent),
	}))
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		return fmt.Errorf("updating bench workflow %q: %w", name, err)
	}
	if _, err := b.client.CreateWorkflow(ctx, connect.NewRequest(&workflowv1.CreateWorkflowRequest{
		Yaml: string(yamlContent),
	})); err != nil {
		return fmt.Errorf("creating bench workflow %q: %w", name, err)
	}
	return nil
}

func (b *benchClient) triggerRun(ctx context.Context, workflowName string) (runID string, err error) {
	resp, err := b.client.TriggerRun(ctx, connect.NewRequest(&workflowv1.TriggerRunRequest{
		WorkflowName: workflowName,
	}))
	if err != nil {
		return "", fmt.Errorf("triggering bench run for %q: %w", workflowName, err)
	}
	return resp.Msg.GetRunId(), nil
}

var errBenchRunNotTerminal = errors.New("bench run has not reached a terminal status")

func (b *benchClient) getRun(ctx context.Context, runID string) (*workflowv1.Run, error) {
	resp, err := b.client.GetRun(ctx, connect.NewRequest(&workflowv1.GetRunRequest{RunId: runID}))
	if err != nil {
		return nil, fmt.Errorf("getting bench run %q: %w", runID, err)
	}
	return resp.Msg, nil
}

// pollInterval/pollTimeout bound the background poller's own wait -- a verification run that
// genuinely takes longer than pollTimeout is surfaced as FAILED with a clear reason rather than
// polled forever; the merge queue entry otherwise never leaves VERIFICATION_STATUS_RUNNING if Bench
// itself hangs or a run is simply misconfigured to never finish.
const (
	pollInterval = 2 * time.Second
	pollTimeout  = 5 * time.Minute
)

// waitForTerminalRun polls GetRun until a terminal status, pollTimeout, or ctx cancellation --
// whichever comes first. Returns the last-seen Run even on timeout, so the caller can still report
// whatever partial step detail Bench had produced.
func (b *benchClient) waitForTerminalRun(ctx context.Context, runID string) (*workflowv1.Run, error) {
	deadline := time.Now().Add(pollTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	run, err := b.getRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	for !isTerminalRunStatus(run.GetStatus()) {
		if time.Now().After(deadline) {
			return run, fmt.Errorf("bench run %q did not reach a terminal status within %s: %w", runID, pollTimeout, errBenchRunNotTerminal)
		}
		select {
		case <-ctx.Done():
			return run, ctx.Err()
		case <-ticker.C:
		}
		run, err = b.getRun(ctx, runID)
		if err != nil {
			return nil, err
		}
	}
	return run, nil
}
