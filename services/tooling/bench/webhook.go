// This file implements Phase 4: the raw HTTP webhook receiver described in
// docs/website/content/docs/architecture/bench-workflow-engine.md's "The webhook
// trigger and status API" section — POST /webhooks/{slug}, HMAC-SHA256-verified,
// 202 { run_id, status } back immediately while the run continues in the
// background (scheduler.go's StartRun).
//
// This is a plain net/http handler, not a Connect/gRPC one, per the doc's own
// explicit reasoning: external callers (CI providers, git hosts, or in this repo's
// case, anything posting to a workflow's configured webhook) send arbitrary JSON,
// not Connect-framed requests.
//
// Mounting approach: onto chassis's own mux, via the same chassis.Rpcer.AddHandler
// primitive WithRPCHandler's RegisterRPC path already uses for Connect handlers —
// not a second net/http.ListenAndServe the way services/core/auth's ext_authz check
// endpoint runs (see that file's serveCheckEndpoint). AddHandler takes an arbitrary
// http.Handler; a webhook receiver already is one, with no gRPC/Connect framing
// requirement that would force a second listener the way auth's ext_authz protocol
// (a distinct Envoy-facing contract) does. Reusing the one mux chassis already runs
// (already CORS-wrapped, already h2c-served) is simpler and one fewer moving part
// to keep alive than standing up bench's own second http.Server — auth's pattern is
// the right call for what auth is doing, but isn't the only established pattern,
// and isn't the better fit here.
//
// One side effect of reusing AddHandler worth calling out: chassis.Rpcer.AddHandler
// always appends its pattern (here, "webhooks") to the service names advertised to
// Blueprint's Synchronize metadata, regardless of the enableReflection argument
// (see pkg/chassis/rpc.go's AddHandler). That's harmless — nothing resolves RPC
// services named "webhooks" — but it does mean Blueprint's registry will list
// "webhooks" as one of the RPC-shaped things Bench serves, which it isn't. Not
// worth a chassis change for.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	webhookPathPrefix = "/webhooks/"
	// maxWebhookBodyBytes bounds how much of a request body the handler will read
	// before giving up — an external caller's request body is untrusted input, and
	// nothing about a workflow trigger payload should ever need to be large.
	maxWebhookBodyBytes = 1 << 20 // 1 MiB

	// blueprintSecretRefPrefix is the prefix Trigger.Webhook.SecretRef is stripped
	// of before being treated as a literal Blueprint KV key — see the doc's YAML
	// example (`secret_ref: blueprint://secrets/bench/course-creation-webhook`) and
	// the brief: "for this phase, treat it as a literal Blueprint KV key."
	blueprintSecretRefPrefix = "blueprint://secrets/"

	defaultSignatureHeader = "X-Bench-Signature"
)

// workflowRunner is the piece of Scheduler the webhook handler and TriggerRun both
// need: begin a run and return immediately, per the doc's synchronous-trigger
// requirement. Satisfied by *Scheduler; kept as an interface so tests can substitute
// a fake without exercising the real DAG executor.
type workflowRunner interface {
	StartRun(ctx context.Context, w *workflowv1.Workflow) (*workflowv1.Run, error)
}

// workflowGetter is the read side of *pgResultStore the webhook handler and rpc.go
// both depend on — kept as a narrow interface (rather than depending on the
// concrete *pgResultStore everywhere) purely so tests can fake it. ListWorkflows
// is what a webhook request resolves its slug against (see slugToWorkflowName
// below) — Phase 10's dynamic-routing requirement (a workflow created/edited
// through the UI must be reachable by webhook immediately, not after a restart)
// ruled out the static map this handler used before.
type workflowGetter interface {
	GetWorkflow(ctx context.Context, name string) (*workflowv1.Workflow, error)
	ListWorkflows(ctx context.Context) ([]*workflowv1.Workflow, error)
}

// secretResolver resolves a Trigger.Webhook.SecretRef to the literal HMAC secret it
// names.
type secretResolver interface {
	Resolve(ctx context.Context, secretRef string) (string, error)
}

// blueprintSecretResolver is the real secretResolver: every secret_ref is treated
// as `blueprint://secrets/<key>` and resolved via Blueprint's KeyValueService.Get —
// the same "talk to another Draft service's RPC directly" pattern
// services/core/auth and services/core/fuse's control_plane package already use for
// their own Blueprint KV lookups (see pkg/chassis/networking.go's withRoute and
// services/core/fuse/control_plane/controller.go's getAuthServiceAddress, which
// this mirrors field-for-field: an empty *kvv1.Value wrapped in an Any as the Get
// request's type witness, the response's Any unmarshaled back into a *kvv1.Value).
type blueprintSecretResolver struct {
	client kvv1Connect.KeyValueServiceClient
}

func newBlueprintSecretResolver(httpClient connect.HTTPClient, entrypoint string) secretResolver {
	return &blueprintSecretResolver{
		client: kvv1Connect.NewKeyValueServiceClient(httpClient, entrypoint),
	}
}

func (r *blueprintSecretResolver) Resolve(ctx context.Context, secretRef string) (string, error) {
	key := strings.TrimPrefix(secretRef, blueprintSecretRefPrefix)
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("secret_ref %q resolved to an empty blueprint key", secretRef)
	}

	witness, err := anypb.New(&kvv1.Value{})
	if err != nil {
		return "", fmt.Errorf("failed to build KV lookup value: %w", err)
	}

	resp, err := r.client.Get(ctx, connect.NewRequest(&kvv1.GetRequest{
		Key:   key,
		Value: witness,
	}))
	if err != nil {
		return "", fmt.Errorf("failed to resolve secret %q from blueprint: %w", secretRef, err)
	}

	value := &kvv1.Value{}
	if err := resp.Msg.GetValue().UnmarshalTo(value); err != nil {
		return "", fmt.Errorf("failed to unmarshal secret %q: %w", secretRef, err)
	}
	if value.GetData() == "" {
		return "", fmt.Errorf("secret %q resolved to an empty value", secretRef)
	}
	return value.GetData(), nil
}

// webhookHandler is the http.Handler behind POST /webhooks/{slug}.
type webhookHandler struct {
	logger    chassis.Logger
	workflows workflowGetter
	scheduler workflowRunner
	secrets   secretResolver
}

func newWebhookHandler(logger chassis.Logger, workflows workflowGetter, scheduler workflowRunner, secrets secretResolver) *webhookHandler {
	return &webhookHandler{
		logger:    logger,
		workflows: workflows,
		scheduler: scheduler,
		secrets:   secrets,
	}
}

// workflowForSlug scans every currently-persisted workflow for one whose
// Trigger.Webhook.Slug matches slug. Deliberately a per-request scan, not a
// cached map: Phase 10's Create/Update/Delete need to take effect on the
// very next webhook delivery, and the workflow count this scans is the same
// "stays small for a long time" scale every other list view in this package
// already assumes (see ui.go's dashboardRunSample comment for the same
// reasoning applied elsewhere). Returns ErrWorkflowNotFound if nothing
// matches, so callers can treat it the same as GetWorkflow's own not-found.
func workflowForSlug(ctx context.Context, workflows workflowGetter, slug string) (*workflowv1.Workflow, error) {
	all, err := workflows.ListWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows while resolving webhook slug %q: %w", slug, err)
	}
	for _, w := range all {
		if w.GetTrigger().GetWebhook().GetSlug() == slug {
			return w, nil
		}
	}
	return nil, ErrWorkflowNotFound
}

// RegisterRPC mounts the webhook handler onto chassis's mux at /webhooks/ — see the
// file comment for why AddHandler (not a second listener) is the right primitive
// here even though this isn't an RPC handler in any Connect/gRPC sense.
func (h *webhookHandler) RegisterRPC(server chassis.Rpcer) {
	server.AddHandler(webhookPathPrefix, h, false)
}

func (h *webhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := strings.Trim(strings.TrimPrefix(r.URL.Path, webhookPathPrefix), "/")
	if slug == "" {
		http.Error(w, "missing webhook slug", http.StatusNotFound)
		return
	}

	ctx := r.Context()

	workflow, err := workflowForSlug(ctx, h.workflows, slug)
	if err != nil {
		if !errors.Is(err, ErrWorkflowNotFound) {
			h.logger.WithError(err).WithField("slug", slug).Error("failed to resolve webhook slug")
		}
		http.Error(w, fmt.Sprintf("no workflow registered for webhook slug %q", slug), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodyBytes+1))
	_ = r.Body.Close()
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	if len(body) > maxWebhookBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err := h.verifySignature(ctx, workflow, r.Header, body); err != nil {
		h.logger.WithError(err).WithField("slug", slug).Warn("rejected webhook: signature verification failed")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	run, err := h.scheduler.StartRun(ctx, workflow)
	if err != nil {
		h.logger.WithError(err).WithField("workflow", workflow.GetName()).Error("failed to start run")
		http.Error(w, "failed to start run", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"run_id": run.GetRunId(),
		"status": "pending",
	})
}

// verifySignature checks the request's signature header against an HMAC-SHA256 of
// the raw request body, keyed by workflow's configured webhook secret. Any failure
// along the way — a workflow with no webhook trigger, no signature_header/secret_ref
// configured, a secret that can't be resolved, or a mismatched signature — is
// reported uniformly and rejected with 401 by the caller (ServeHTTP), matching the
// brief: "reject the request with 401 if the signature doesn't match or the secret
// can't be resolved."
func (h *webhookHandler) verifySignature(ctx context.Context, workflow *workflowv1.Workflow, header http.Header, body []byte) error {
	webhook := workflow.GetTrigger().GetWebhook()
	if webhook == nil {
		return errors.New("workflow has no webhook trigger configured")
	}

	headerName := webhook.GetSignatureHeader()
	if headerName == "" {
		headerName = defaultSignatureHeader
	}
	got := header.Get(headerName)
	if got == "" {
		return fmt.Errorf("missing %s header", headerName)
	}

	secretRef := webhook.GetSecretRef()
	if secretRef == "" {
		return errors.New("workflow webhook trigger has no secret_ref configured")
	}
	secret, err := h.secrets.Resolve(ctx, secretRef)
	if err != nil {
		return fmt.Errorf("failed to resolve webhook secret: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(strings.ToLower(got)), []byte(want)) {
		return errors.New("signature mismatch")
	}
	return nil
}
