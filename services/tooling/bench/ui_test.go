package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// --- fakes -----------------------------------------------------------------------------------

// fakeStore is uiStore in memory, so the page tests need no Postgres.
type fakeStore struct {
	mu         sync.Mutex
	workflows  map[string]*workflowv1.Workflow
	runs       []*workflowv1.Run
	registries []*settingsv1.PluginRegistry
}

func newFakeStore() *fakeStore {
	return &fakeStore{workflows: map[string]*workflowv1.Workflow{}}
}

func (s *fakeStore) GetWorkflow(_ context.Context, name string) (*workflowv1.Workflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w, ok := s.workflows[name]; ok {
		return w, nil
	}
	return nil, ErrWorkflowNotFound
}

func (s *fakeStore) UpsertWorkflow(_ context.Context, w *workflowv1.Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflows[w.GetName()] = w
	return nil
}

func (s *fakeStore) DeleteWorkflow(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workflows[name]; !ok {
		return ErrWorkflowNotFound
	}
	delete(s.workflows, name)
	return nil
}

func (s *fakeStore) ListWorkflows(context.Context) ([]*workflowv1.Workflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*workflowv1.Workflow
	for _, w := range s.workflows {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetName() < out[j].GetName() })
	return out, nil
}

func (s *fakeStore) GetRun(_ context.Context, id string) (*workflowv1.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		if r.GetRunId() == id {
			return r, nil
		}
	}
	return nil, ErrRunNotFound
}

func recordOf(r *workflowv1.Run) runRecord {
	rec := runRecord{RunID: r.GetRunId(), Workflow: r.GetWorkflowName(), Status: r.GetStatus()}
	if r.GetStartedAt() != nil {
		rec.Started = r.GetStartedAt().AsTime()
	}
	if r.GetFinishedAt() != nil {
		rec.Finished = r.GetFinishedAt().AsTime()
	}
	return rec
}

// sortedRecords is every run newest first.
func (s *fakeStore) sortedRecords() []runRecord {
	out := make([]runRecord, 0, len(s.runs))
	for _, r := range s.runs {
		out = append(out, recordOf(r))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Started.Equal(out[j].Started) {
			return out[i].Started.After(out[j].Started)
		}
		return out[i].RunID > out[j].RunID
	})
	return out
}

func (s *fakeStore) RunRecords(_ context.Context, f runFilter, pageSize int, pageToken string) ([]runRecord, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var matched []runRecord
	for _, r := range s.sortedRecords() {
		if f.Workflow != "" && r.Workflow != f.Workflow {
			continue
		}
		if vals := statusValues(f.Status); vals != nil {
			ok := false
			for _, v := range vals {
				ok = ok || v == r.Status.String()
			}
			if !ok {
				continue
			}
		}
		if !f.Since.IsZero() && r.Started.Before(f.Since) {
			continue
		}
		matched = append(matched, r)
	}
	offset := 0
	if pageToken != "" {
		fmt.Sscanf(pageToken, "%d", &offset)
	}
	if offset > len(matched) {
		offset = len(matched)
	}
	matched = matched[offset:]
	next := ""
	if pageSize > 0 && len(matched) > pageSize {
		next = fmt.Sprintf("%d", offset+pageSize)
		matched = matched[:pageSize]
	}
	return matched, next, nil
}

func (s *fakeStore) LastRunRecords(_ context.Context, n int) (map[string][]runRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string][]runRecord{}
	for _, r := range s.sortedRecords() {
		if len(out[r.Workflow]) < n {
			out[r.Workflow] = append(out[r.Workflow], r)
		}
	}
	return out, nil
}

func (s *fakeStore) RunCounts(_ context.Context, since time.Time) (runCounts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c runCounts
	for _, r := range s.sortedRecords() {
		if r.Started.Before(since) {
			continue
		}
		c.Total++
		switch {
		case r.Passed():
			c.Passed++
		case r.Failed():
			c.Failed++
		case r.InFlight():
			c.InFlight++
		}
	}
	return c, nil
}

func (s *fakeStore) FailedSteps(_ context.Context, ids []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for _, id := range ids {
		for _, r := range s.runs {
			if r.GetRunId() != id {
				continue
			}
			for _, st := range r.GetSteps() {
				if st.GetStatus() == workflowv1.StepStatus_STEP_STATUS_FAILED {
					out[id] = st.GetStepName()
					break
				}
			}
		}
	}
	return out, nil
}

func (s *fakeStore) ListPluginRegistries(context.Context) ([]*settingsv1.PluginRegistry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*settingsv1.PluginRegistry(nil), s.registries...), nil
}

func (s *fakeStore) GetPluginRegistry(_ context.Context, name string) (*settingsv1.PluginRegistry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.registries {
		if r.GetName() == name {
			return r, nil
		}
	}
	return nil, ErrPluginRegistryNotFound
}

func (s *fakeStore) UpsertPluginRegistry(_ context.Context, r *settingsv1.PluginRegistry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registries = append(s.registries, r)
	return nil
}

func (s *fakeStore) DeletePluginRegistry(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.registries {
		if r.GetName() == name {
			s.registries = append(s.registries[:i], s.registries[i+1:]...)
			return nil
		}
	}
	return ErrPluginRegistryNotFound
}

// uiRunner starts a run that is still going.
type uiRunner struct {
	store *fakeStore
	now   time.Time
	n     int
}

func (f *uiRunner) StartRun(_ context.Context, w *workflowv1.Workflow) (*workflowv1.Run, error) {
	f.n++
	run := &workflowv1.Run{
		RunId:        fmt.Sprintf("started-%d", f.n),
		WorkflowName: w.GetName(),
		Status:       workflowv1.RunStatus_RUN_STATUS_RUNNING,
		StartedAt:    timestamppb.New(f.now),
	}
	f.store.mu.Lock()
	f.store.runs = append(f.store.runs, run)
	f.store.mu.Unlock()
	return run, nil
}

// --- seed ------------------------------------------------------------------------------------

func testStep(name, uses string) *workflowv1.Step {
	return &workflowv1.Step{Name: name, Uses: uses}
}

func testWorkflow(name, desc, slug string, steps ...*workflowv1.Step) *workflowv1.Workflow {
	w := &workflowv1.Workflow{Name: name, Description: desc, Steps: steps}
	if slug != "" {
		w.Trigger = &workflowv1.Trigger{Webhook: &workflowv1.WebhookTrigger{Slug: slug}}
	}
	return w
}

func testRun(id, workflow string, status workflowv1.RunStatus, started time.Time, took time.Duration, steps ...*workflowv1.StepResult) *workflowv1.Run {
	r := &workflowv1.Run{RunId: id, WorkflowName: workflow, Status: status, StartedAt: timestamppb.New(started), Steps: steps}
	if took > 0 {
		r.FinishedAt = timestamppb.New(started.Add(took))
	}
	return r
}

func stepResult(name string, status workflowv1.StepStatus, started time.Time, took time.Duration) *workflowv1.StepResult {
	r := &workflowv1.StepResult{StepName: name, Status: status}
	if !started.IsZero() {
		r.StartedAt = timestamppb.New(started)
		if took > 0 {
			r.FinishedAt = timestamppb.New(started.Add(took))
		}
	}
	return r
}

var testNow = time.Date(2026, 9, 26, 2, 0, 0, 0, time.UTC)

// seededStore is the bench of the mockups: five workflows, one failing, one running.
func seededStore() *fakeStore {
	s := newFakeStore()
	const grpc, http_ = "foundry://grpc-call@v1", "foundry://http-call@v1"
	// 21 steps: register-route, then call-1, wait-1 … call-10, wait-10. call-7 is the 14th.
	tenX := []*workflowv1.Step{testStep("register-route", grpc)}
	for i := 1; i <= 10; i++ {
		tenX = append(tenX, testStep(fmt.Sprintf("call-%d", i), http_), testStep(fmt.Sprintf("wait-%d", i), "builtin://sleep"))
	}
	for _, w := range []*workflowv1.Workflow{
		testWorkflow("fuse-proxy-e2e-10x", "Register a route with Fuse, call it through Envoy 10 times", "", tenX...),
		testWorkflow("crud-e2e", "Create a Name via CrudService through the real Envoy proxy", "crud-e2e", testStep("create-name", http_), testStep("query-name", grpc)),
		testWorkflow("fuse-proxy-e2e", "Register a route with Fuse and call it through Envoy", "", testStep("register", grpc), testStep("call", http_), testStep("cleanup", grpc)),
		testWorkflow("lineman-loop-smoke", "Create a loop in Lineman", "", testStep("create-loop", grpc)),
		testWorkflow("slack-notify-canary", "Post a message through slack-notify", "", testStep("post", http_)),
	} {
		s.workflows[w.GetName()] = w
	}

	pass, fail, run := workflowv1.RunStatus_RUN_STATUS_PASSED, workflowv1.RunStatus_RUN_STATUS_FAILED, workflowv1.RunStatus_RUN_STATUS_RUNNING
	ok, bad, skip := workflowv1.StepStatus_STEP_STATUS_PASSED, workflowv1.StepStatus_STEP_STATUS_FAILED, workflowv1.StepStatus_STEP_STATUS_SKIPPED

	// fuse-proxy-e2e-10x: eleven passes, then a failure at its 14th step (1h ago).
	for i := 0; i < 11; i++ {
		s.runs = append(s.runs, testRun(fmt.Sprintf("tenx-%02d", i), "fuse-proxy-e2e-10x", pass, testNow.Add(-time.Duration(20-i)*time.Hour), 51*time.Second))
	}
	var failing []*workflowv1.StepResult
	at := testNow.Add(-time.Hour)
	for i, st := range tenX {
		switch {
		case i < 13:
			failing = append(failing, stepResult(st.GetName(), ok, at, 200*time.Millisecond))
		case i == 13:
			r := stepResult(st.GetName(), bad, at, 2010*time.Millisecond)
			r.Error = "expect.status: wanted 200, got 503"
			r.Request, _ = structpb.NewStruct(map[string]interface{}{"url": "http://files.draft.localhost:10000/health", "timeout": "2s"})
			r.Result, _ = structpb.NewStruct(map[string]interface{}{"status": 503, "body": "upstream connect error"})
			failing = append(failing, r)
		default:
			failing = append(failing, stepResult(st.GetName(), skip, time.Time{}, 0))
		}
	}
	s.runs = append(s.runs, testRun("tenx-fail", "fuse-proxy-e2e-10x", fail, at, 38*time.Second, failing...))

	// crud-e2e: passes, and one run in flight.
	for i := 0; i < 11; i++ {
		s.runs = append(s.runs, testRun(fmt.Sprintf("crud-%02d", i), "crud-e2e", pass, testNow.Add(-time.Duration(22-i)*time.Hour), 800*time.Millisecond))
	}
	s.runs = append(s.runs, testRun("crud-live", "crud-e2e", run, testNow.Add(-1100*time.Millisecond), 0,
		stepResult("create-name", ok, testNow.Add(-time.Second), 500*time.Millisecond),
		stepResult("query-name", workflowv1.StepStatus_STEP_STATUS_RUNNING, testNow.Add(-500*time.Millisecond), 0)))

	for i := 0; i < 12; i++ {
		s.runs = append(s.runs, testRun(fmt.Sprintf("proxy-%02d", i), "fuse-proxy-e2e", pass, testNow.Add(-time.Duration(23-i)*time.Hour), 1800*time.Millisecond))
		s.runs = append(s.runs, testRun(fmt.Sprintf("loop-%02d", i), "lineman-loop-smoke", pass, testNow.Add(-time.Duration(23-i)*time.Hour-time.Minute), 1200*time.Millisecond))
	}
	for i := 0; i < 3; i++ {
		s.runs = append(s.runs, testRun(fmt.Sprintf("canary-%d", i), "slack-notify-canary", pass, testNow.Add(-time.Duration(60+i)*time.Hour), 400*time.Millisecond))
	}
	s.registries = []*settingsv1.PluginRegistry{{Name: "foundry", Address: "http://localhost:9301"}}
	return s
}

func newTestUI(t *testing.T, s *fakeStore) *httptest.Server {
	t.Helper()
	h := newUIHandler(noopLogger{}, s, &uiRunner{store: s, now: testNow}, http.DefaultClient)
	h.now = func() time.Time { return testNow }
	srv := httptest.NewServer(h.mux())
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, b.String()
}

// postForm posts without following the redirect, returning the status, Location and body.
func postForm(t *testing.T, srv *httptest.Server, path string, form url.Values) (int, string, string) {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.PostForm(srv.URL+path, form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, resp.Header.Get("Location"), b.String()
}

func mustContain(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(body, sub) {
			t.Errorf("missing %q in:\n%s", sub, body)
		}
	}
}

func mustNotContain(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if strings.Contains(body, sub) {
			t.Errorf("unexpected %q in:\n%s", sub, body)
		}
	}
}

// --- overview --------------------------------------------------------------------------------

func TestOverviewIsTheMockupsPage(t *testing.T) {
	srv := newTestUI(t, seededStore())
	code, body := get(t, srv, "/")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body,
		`<title>Overview · bench</title>`,
		"Is it green?",
		`class="d-verdict"`, "<b>Mostly.</b>",
		"3 of 5 workflows passed their last run.", // crud-e2e is still running, so not counted as passed
		"fuse-proxy-e2e-10x failed at step 14 at 01:00 UTC.",
		`d-seg`, `aria-current="true">24h`,
		"Pass rate · 24h", "Running now", "Failing workflows", "Median run",
		`d-tag d-tag--ca`, ">webhook<", ">manual<",
		"Failed · step 14",
		`<button class="d-btn d-btn--sm" type="submit" disabled`, // crud-e2e cannot be re-run while it runs
		`d-strip`, `is-none`, // slack-notify-canary has only 3 runs: the rest of its strip is "never run"
		`hx-get="/partials/workflows"`,
		"1 run in flight", // topbar pill
		`href="/workflows/new"`,
	)
	// The rail: counts on All workflows, Recent runs, Failed and the registry.
	mustContain(t, body, `<span class="d-count">5</span>`, `d-count d-count--err`)
	mustNotContain(t, body, "cdn.jsdelivr", "tailwind", "daisyui", "Webhook secrets")
}

func TestOverviewWindowsAndFilters(t *testing.T) {
	srv := newTestUI(t, seededStore())

	_, body := get(t, srv, "/?filter=failing")
	mustContain(t, body, `href="/workflows/fuse-proxy-e2e-10x"`)
	mustNotContain(t, body, `href="/workflows/lineman-loop-smoke"`, `href="/workflows/crud-e2e"`)

	_, body = get(t, srv, "/?filter=webhook")
	mustContain(t, body, `href="/workflows/crud-e2e"`)
	mustNotContain(t, body, `href="/workflows/fuse-proxy-e2e-10x"`)

	_, body = get(t, srv, "/?q=lineman")
	mustContain(t, body, `href="/workflows/lineman-loop-smoke"`)
	mustNotContain(t, body, `href="/workflows/crud-e2e"`)

	_, body = get(t, srv, "/?q=nothing-matches")
	mustContain(t, body, "No workflow matches.")

	_, body = get(t, srv, "/?window=1h")
	mustContain(t, body, `aria-current="true">1h`, "Pass rate · 1h")

	// An unknown window falls back to the default rather than erroring.
	code, body := get(t, srv, "/?window=eternity")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, `aria-current="true">24h`)

	// /workflows is the same page, with its own breadcrumb.
	_, body = get(t, srv, "/workflows")
	mustContain(t, body, "Is it green?", "<span>Workflows</span>")
}

func TestWorkflowTableFragmentHasNoShell(t *testing.T) {
	srv := newTestUI(t, seededStore())
	_, body := get(t, srv, "/partials/workflows?q=crud")
	mustContain(t, body, `id="wf-table"`, "crud-e2e")
	mustNotContain(t, body, "<html", "d-rail", "fuse-proxy-e2e-10x")
}

func TestOverviewWithNothingLoaded(t *testing.T) {
	srv := newTestUI(t, newFakeStore())
	code, body := get(t, srv, "/")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, "Empty bench.", "d-verdict--idle", "No workflows yet.", "Idle")
}

// --- runs ------------------------------------------------------------------------------------

func TestRunsListFiltersAndPages(t *testing.T) {
	srv := newTestUI(t, seededStore())

	_, body := get(t, srv, "/runs")
	mustContain(t, body, "Recent runs", `d-table`, "Older runs →", `href="/runs" aria-current="page"`)

	_, body = get(t, srv, "/runs?status=failed")
	mustContain(t, body, "fuse-proxy-e2e-10x", "Failed · call-7", `href="/runs?status=failed" aria-current="page"`)
	mustNotContain(t, body, "Older runs", ">crud-e2e<")

	_, body = get(t, srv, "/runs?status=running")
	mustContain(t, body, ">crud-e2e<", "Running")

	_, body = get(t, srv, "/runs?workflow=nothing")
	mustContain(t, body, "No runs of nothing match.")

	// The second page is reachable from the first.
	_, first := get(t, srv, "/runs")
	i := strings.Index(first, `href="/runs?page=`)
	if i < 0 {
		t.Fatalf("no older-runs link in:\n%s", first)
	}
	link := first[i+len(`href="`):]
	link = link[:strings.Index(link, `"`)]
	code, second := get(t, srv, strings.ReplaceAll(link, "&amp;", "&"))
	if code != 200 {
		t.Fatalf("second page: %d", code)
	}
	mustContain(t, second, "d-table")
	mustNotContain(t, second, "Older runs →") // 51 runs: the second page is the last
}

// --- a run -----------------------------------------------------------------------------------

func TestFailedRunShowsWhereItFailed(t *testing.T) {
	srv := newTestUI(t, seededStore())
	code, body := get(t, srv, "/runs/tenx-fail")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body,
		"Run tenx-fail", "fuse-proxy-e2e-10x",
		`d-tag d-tag--err d-tag--solid`,
		`class="d-trace"`, `class="d-step is-fail"`,
		`class="d-panel d-fail"`, "expect.status: wanted 200, got 503",
		"http://files.draft.localhost:10000/health", "upstream connect error", // request and response side by side
		`<details class="d-step is-group`, "steps passed", "steps skipped", // long quiet stretches fold up
		"13 passed · 1 failed · 7 skipped",
		`d-strip d-strip--stretch`,
		"Last 12 runs", "Re-run", "Definition",
	)
	// A finished run does not poll.
	mustNotContain(t, body, "hx-trigger")
	mustContain(t, body, "off · run finished")
}

func TestRunningRunPollsAndTheFragmentStops(t *testing.T) {
	store := seededStore()
	srv := newTestUI(t, store)

	_, page := get(t, srv, "/runs/crud-live")
	mustContain(t, page, `hx-get="/partials/runs/crud-live"`, `hx-trigger="every 2s"`, "on · every 2s", `d-step is-run`)

	_, frag := get(t, srv, "/partials/runs/crud-live")
	mustContain(t, frag, `hx-trigger="every 2s"`, `id="run-polling"`, `hx-swap-oob="true"`)
	mustNotContain(t, frag, "<html", "d-rail")

	// Once the run finishes the next poll's fragment carries no trigger, so polling ends by itself.
	for _, r := range store.runs {
		if r.GetRunId() == "crud-live" {
			r.Status = workflowv1.RunStatus_RUN_STATUS_PASSED
			r.FinishedAt = timestamppb.New(testNow)
		}
	}
	_, frag = get(t, srv, "/partials/runs/crud-live")
	mustNotContain(t, frag, "hx-trigger")
	mustContain(t, frag, "off · run finished")
}

func TestUnknownRunIsNotFound(t *testing.T) {
	srv := newTestUI(t, seededStore())
	code, body := get(t, srv, "/runs/nope")
	if code != 404 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, "Not on the bench", `No run with id &#34;nope&#34;.`, "d-rail")
	if code, _ := get(t, srv, "/partials/runs/nope"); code != 404 {
		t.Fatalf("fragment status %d", code)
	}
}

func TestGroupStepsFoldsLongQuietStretches(t *testing.T) {
	mk := func(states ...string) []stepRow {
		var out []stepRow
		for i, s := range states {
			out = append(out, stepRow{No: i + 1, Name: fmt.Sprintf("s%d", i+1), State: s})
		}
		return out
	}
	items := groupSteps(mk("ok", "ok", "ok", "err", "skip", "skip", "skip", "skip", "skip"))
	if len(items) != 5 { // three passes stay separate (not more than 3), the failure, one skipped group
		t.Fatalf("got %d items: %+v", len(items), items)
	}
	last := items[len(items)-1]
	if !last.Group || !last.Skipped || last.Range != "05–09" || last.Summary != "5 steps skipped" {
		t.Fatalf("skipped group wrong: %+v", last)
	}

	items = groupSteps(mk("ok", "ok", "ok", "ok", "err"))
	if !items[0].Group || items[0].Summary != "4 steps passed" || items[0].Names != "s1 … s4" {
		t.Fatalf("passed group wrong: %+v", items[0])
	}
	if items[1].Group || items[1].Step.State != "err" {
		t.Fatalf("a failure must never be folded: %+v", items[1])
	}
}

// --- workflow pages --------------------------------------------------------------------------

func TestWorkflowDetail(t *testing.T) {
	srv := newTestUI(t, seededStore())
	code, body := get(t, srv, "/workflows/crud-e2e")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body,
		"<h1>crud-e2e</h1>", "POST /webhooks/crud-e2e",
		`href="/workflows/crud-e2e/edit"`,
		`data-confirm="Delete crud-e2e?`,
		`action="/workflows/crud-e2e/run"`,
		`d-code`, "create-name", // the definition, highlighted
		"Last 12 runs", `href="/runs/crud-live"`,
	)
	if code, _ := get(t, srv, "/workflows/nope"); code != 404 {
		t.Fatalf("unknown workflow status %d", code)
	}
}

func TestRunNowRedirectsToTheRun(t *testing.T) {
	srv := newTestUI(t, seededStore())
	code, loc, _ := postForm(t, srv, "/workflows/crud-e2e/run", nil)
	if code != http.StatusSeeOther || loc != "/runs/started-1" {
		t.Fatalf("got %d %q", code, loc)
	}
}

func TestDeleteWorkflow(t *testing.T) {
	store := seededStore()
	srv := newTestUI(t, store)
	code, loc, _ := postForm(t, srv, "/workflows/crud-e2e/delete", nil)
	if code != http.StatusSeeOther || loc != "/workflows" {
		t.Fatalf("got %d %q", code, loc)
	}
	if _, err := store.GetWorkflow(context.Background(), "crud-e2e"); err == nil {
		t.Fatal("workflow still there")
	}
	// Its runs stay.
	if _, err := store.GetRun(context.Background(), "crud-live"); err != nil {
		t.Fatalf("run history was deleted: %v", err)
	}
}

func TestEditorPages(t *testing.T) {
	srv := newTestUI(t, seededStore())

	code, body := get(t, srv, "/workflows/new")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body,
		`<title>New workflow · bench</title>`,
		`id="workflow-yaml"`, `data-editor`, `data-gutter="wf-gutter"`, `class="d-editor-code"`,
		`hx-post="/workflows/validate"`,
		`id="wf-pipeline"`, `id="wf-problems"`, `id="wf-status"`,
		`data-yaml-field="metadata.name"`,
		"my-workflow", "bench://grpc-call@v1",
		`class="d-kbd">⌘S</span> save`, // hints in the status bar
		`id="wf-cursor"`, `id="wf-lines"`,
		`form="wf-form"`, // topbar Save submits the form
		`aria-label="Plugin search"`, `hx-get="/workflows/plugins/search"`,
	)
	mustContain(t, body, "Valid") // the starter template is a valid workflow
	mustNotContain(t, body, " readonly")

	code, body = get(t, srv, "/workflows/crud-e2e/edit")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, `<title>Edit crud-e2e · bench</title>`, `action="/workflows/crud-e2e"`, ` readonly`, "A workflow cannot be renamed.", "create-name")
}

func TestValidateReturnsOutOfBandFeedback(t *testing.T) {
	srv := newTestUI(t, seededStore())
	bad := "metadata:\n  name: x\nsteps:\n  - name: a\n    usse: bench://grpc-call@v1\n"
	code, _, body := postForm(t, srv, "/workflows/validate", url.Values{"yaml": {bad}})
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body,
		`id="wf-pipeline"`, `id="wf-problems"`, `id="wf-status"`, `hx-swap-oob="true"`,
		`d-tag d-tag--err`, `d-tag d-tag--warn`,
		"usse is not a field of a step. Did you mean uses?",
		"uses is required.",
		`data-error-lines-for="workflow-yaml">4`, // the step's line is marked in the gutter
		"1 problem",
	)
	mustNotContain(t, body, "<html")

	_, _, body = postForm(t, srv, "/workflows/validate", url.Values{"yaml": {defaultWorkflowTemplate}})
	mustContain(t, body, "Valid", "Nothing to fix.")
}

func TestSaveCreatesAndCanRun(t *testing.T) {
	store := seededStore()
	srv := newTestUI(t, store)
	doc := "apiVersion: bench/v1\nkind: Workflow\nmetadata:\n  name: brand-new\nsteps:\n  - name: a\n    uses: bench://grpc-call@v1\n"

	code, loc, _ := postForm(t, srv, "/workflows", url.Values{"yaml": {doc}})
	if code != http.StatusSeeOther || loc != "/workflows/brand-new" {
		t.Fatalf("save: got %d %q", code, loc)
	}
	if _, err := store.GetWorkflow(context.Background(), "brand-new"); err != nil {
		t.Fatalf("not saved: %v", err)
	}

	// ⌘↵ sets run=1: the update saves and lands on the new run.
	code, loc, _ = postForm(t, srv, "/workflows/brand-new", url.Values{"yaml": {doc}, "run": {"1"}})
	if code != http.StatusSeeOther || loc != "/runs/started-1" {
		t.Fatalf("save and run: got %d %q", code, loc)
	}
}

func TestSaveKeepsWhatWasTypedWhenItFails(t *testing.T) {
	srv := newTestUI(t, seededStore())

	// Invalid YAML: the editor comes back with the text and the reason.
	bad := "metadata:\n  name: x\nsteps:\n  - name: a\n    uses: nonsense\n"
	code, _, body := postForm(t, srv, "/workflows", url.Values{"yaml": {bad}})
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, "uses: nonsense", `d-problem`, "must start with bench:// or foundry://")

	// A name that is taken is not a YAML problem, but it is still shown.
	dup := "metadata:\n  name: crud-e2e\nsteps:\n  - name: a\n    uses: bench://grpc-call@v1\n"
	code, _, body = postForm(t, srv, "/workflows", url.Values{"yaml": {dup}})
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, "workflow already exists", "crud-e2e")

	// Renaming through an update is refused.
	code, _, body = postForm(t, srv, "/workflows/crud-e2e", url.Values{"yaml": {strings.Replace(dup, "crud-e2e", "other", 1)}})
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, "does not match the workflow being updated")
}

// --- settings and the rest -------------------------------------------------------------------

func TestSettings(t *testing.T) {
	srv := newTestUI(t, seededStore())
	_, body := get(t, srv, "/settings")
	mustContain(t, body, "Plugin registries", "localhost:9301", `data-confirm="Remove registry foundry?`, `action="/settings"`, `href="/settings" aria-current="page"`)

	// An address nothing answers at is refused, and what was typed comes back.
	_, _, body = postForm(t, srv, "/settings", url.Values{"name": {"nope"}, "address": {"http://127.0.0.1:1"}})
	mustContain(t, body, `class="d-alert d-alert--err`, "could not reach", `value="nope"`, `value="http://127.0.0.1:1"`)

	code, loc, _ := postForm(t, srv, "/settings/foundry/delete", nil)
	if code != http.StatusSeeOther || loc != "/settings" {
		t.Fatalf("delete: %d %q", code, loc)
	}
}

func TestStaticAssetsComeFromTheDesignSystem(t *testing.T) {
	srv := newTestUI(t, seededStore())
	for _, path := range []string{"/static/draft/draft.css", "/static/draft/draft.js", "/static/draft/htmx.min.js", "/static/draft/fonts.css"} {
		if code, _ := get(t, srv, path); code != 200 {
			t.Errorf("%s: %d", path, code)
		}
	}
}

func TestUnknownPathIsAStyledNotFound(t *testing.T) {
	srv := newTestUI(t, seededStore())
	code, body := get(t, srv, "/nowhere")
	if code != 404 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, "Not on the bench", "Nothing lives at /nowhere.", `href="/"`)
}
