package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
)

// fakeWorkflowGetter is a scripted workflowGetter: a fixed set of workflows keyed
// by name, standing in for *pgResultStore's GetWorkflow without a real Postgres
// instance.
type fakeWorkflowGetter struct {
	workflows map[string]*workflowv1.Workflow
}

func (g *fakeWorkflowGetter) GetWorkflow(ctx context.Context, name string) (*workflowv1.Workflow, error) {
	w, ok := g.workflows[name]
	if !ok {
		return nil, ErrWorkflowNotFound
	}
	return w, nil
}

// ListWorkflows makes fakeWorkflowGetter satisfy the full workflowGetter
// interface -- webhook routing (webhook.go's workflowForSlug) scans this to
// resolve a slug, rather than being handed a separate slug->name map, so
// tests only need to declare the workflows themselves, not a redundant
// routing table alongside them.
func (g *fakeWorkflowGetter) ListWorkflows(ctx context.Context) ([]*workflowv1.Workflow, error) {
	out := make([]*workflowv1.Workflow, 0, len(g.workflows))
	for _, w := range g.workflows {
		out = append(out, w)
	}
	return out, nil
}

// fakeSecretResolver is a scripted secretResolver: resolves any secretRef to a
// fixed secret, or fails if configured to.
type fakeSecretResolver struct {
	secret string
	err    error
}

func (r *fakeSecretResolver) Resolve(ctx context.Context, secretRef string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.secret, nil
}

const testWebhookSecret = "shh-its-a-secret"

func webhookTestWorkflow(name, slug string) *workflowv1.Workflow {
	return &workflowv1.Workflow{
		Name: name,
		Trigger: &workflowv1.Trigger{
			Webhook: &workflowv1.WebhookTrigger{
				Slug:            slug,
				SignatureHeader: defaultSignatureHeader,
				SecretRef:       "blueprint://secrets/bench/" + slug,
			},
		},
		Steps: []*workflowv1.Step{{Name: "step-a", Uses: "bench://grpc-call@v1"}},
	}
}

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func newTestWebhookHandler(workflows map[string]*workflowv1.Workflow, runner *fakeRunner, secret string) *webhookHandler {
	return newWebhookHandler(
		noopLogger{},
		&fakeWorkflowGetter{workflows: workflows},
		runner,
		&fakeSecretResolver{secret: secret},
	)
}

func TestWebhook_ValidSignature_StartsRunAndReturns202(t *testing.T) {
	w := webhookTestWorkflow("webhook-test-workflow", "webhook-test-slug")
	runner := &fakeRunner{run: &workflowv1.Run{RunId: "webhook-test-run-id", WorkflowName: w.GetName(), Status: workflowv1.RunStatus_RUN_STATUS_PENDING}}
	h := newTestWebhookHandler(map[string]*workflowv1.Workflow{w.GetName(): w}, runner, testWebhookSecret)

	body := []byte(`{"inputs":{}}`)
	req := httptest.NewRequest(http.MethodPost, webhookPathPrefix+"webhook-test-slug", strings.NewReader(string(body)))
	req.Header.Set(defaultSignatureHeader, signBody(testWebhookSecret, body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to unmarshal response body %q: %v", rec.Body.String(), err)
	}
	if got["run_id"] != "webhook-test-run-id" {
		t.Errorf("run_id = %q, want %q", got["run_id"], "webhook-test-run-id")
	}
	if got["status"] != "pending" {
		t.Errorf("status = %q, want %q", got["status"], "pending")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected StartRun to be called once, got %d calls", len(runner.calls))
	}
}

func TestWebhook_BadSignature_Returns401AndDoesNotStartRun(t *testing.T) {
	w := webhookTestWorkflow("webhook-test-workflow-bad-sig", "webhook-test-slug-bad-sig")
	runner := &fakeRunner{run: &workflowv1.Run{RunId: "should-not-be-used"}}
	h := newTestWebhookHandler(map[string]*workflowv1.Workflow{w.GetName(): w}, runner, testWebhookSecret)

	body := []byte(`{"inputs":{}}`)
	req := httptest.NewRequest(http.MethodPost, webhookPathPrefix+"webhook-test-slug-bad-sig", strings.NewReader(string(body)))
	req.Header.Set(defaultSignatureHeader, "0000000000000000000000000000000000000000000000000000000000000000")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected StartRun not to be called for a bad signature, got %d calls", len(runner.calls))
	}
}

func TestWebhook_MissingSignatureHeader_Returns401(t *testing.T) {
	w := webhookTestWorkflow("webhook-test-workflow-no-sig", "webhook-test-slug-no-sig")
	runner := &fakeRunner{}
	h := newTestWebhookHandler(map[string]*workflowv1.Workflow{w.GetName(): w}, runner, testWebhookSecret)

	req := httptest.NewRequest(http.MethodPost, webhookPathPrefix+"webhook-test-slug-no-sig", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected StartRun not to be called with no signature header, got %d calls", len(runner.calls))
	}
}

func TestWebhook_UnresolvableSecret_Returns401(t *testing.T) {
	w := webhookTestWorkflow("webhook-test-workflow-bad-secret", "webhook-test-slug-bad-secret")
	runner := &fakeRunner{}
	h := newWebhookHandler(
		noopLogger{},
		&fakeWorkflowGetter{workflows: map[string]*workflowv1.Workflow{w.GetName(): w}},
		runner,
		&fakeSecretResolver{err: errors.New("blueprint unreachable")},
	)

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, webhookPathPrefix+"webhook-test-slug-bad-secret", strings.NewReader(string(body)))
	req.Header.Set(defaultSignatureHeader, signBody("irrelevant", body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected StartRun not to be called when the secret can't be resolved, got %d calls", len(runner.calls))
	}
}

func TestWebhook_UnknownSlug_Returns404(t *testing.T) {
	runner := &fakeRunner{}
	h := newTestWebhookHandler(map[string]*workflowv1.Workflow{}, runner, testWebhookSecret)

	req := httptest.NewRequest(http.MethodPost, webhookPathPrefix+"does-not-exist", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected StartRun not to be called for an unknown slug, got %d calls", len(runner.calls))
	}
}

func TestWebhook_WrongMethod_Returns405(t *testing.T) {
	h := newTestWebhookHandler(map[string]*workflowv1.Workflow{}, &fakeRunner{}, testWebhookSecret)

	req := httptest.NewRequest(http.MethodGet, webhookPathPrefix+"anything", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
