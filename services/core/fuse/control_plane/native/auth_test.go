package native

import (
	"net/http"
	"net/http/httptest"
	"testing"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
)

func TestAuthRequired(t *testing.T) {
	cases := []struct {
		name string
		auth *ntv1.RouteAuth
		want bool
	}{
		{"nil auth", nil, false},
		{"disabled", &ntv1.RouteAuth{Enabled: false, Policy: ntv1.AuthPolicy_AUTH_POLICY_AUTHENTICATED}, false},
		{"bypass", &ntv1.RouteAuth{Enabled: true, Policy: ntv1.AuthPolicy_AUTH_POLICY_BYPASS}, false},
		{"authenticated", &ntv1.RouteAuth{Enabled: true, Policy: ntv1.AuthPolicy_AUTH_POLICY_AUTHENTICATED}, true},
		{"groups", &ntv1.RouteAuth{Enabled: true, Policy: ntv1.AuthPolicy_AUTH_POLICY_GROUPS}, true},
		{"scopes", &ntv1.RouteAuth{Enabled: true, Policy: ntv1.AuthPolicy_AUTH_POLICY_SCOPES}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := authRequired(&ntv1.Route{Auth: c.auth})
			if got != c.want {
				t.Errorf("authRequired() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestAuthMiddleware_OnlyForwardsAuthorization locks in the empirically
// verified wire contract (see auth.go's package comment): the check
// request Envoy's ext_authz filter actually sends forwards the original
// method and path but only the Authorization header -- not Cookie,
// User-Agent, or any custom header.
func TestAuthMiddleware_OnlyForwardsAuthorization(t *testing.T) {
	var gotMethod, gotPath string
	var gotHeaders http.Header
	authSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotHeaders = r.Method, r.URL.Path, r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer authSvc.Close()

	req := httptest.NewRequest(http.MethodPost, "/examples.auth.v1.AuthExampleService/Secret", nil)
	req.Header.Set("Authorization", "Bearer my-test-token")
	req.Header.Set("Cookie", "session=xyz789")
	req.Header.Set("User-Agent", "my-test-agent")
	req.Header.Set("X-Custom-Test-Header", "hello-custom")
	w := httptest.NewRecorder()

	mw := newAuthMiddleware()
	if allow := mw.check(w, req, authSvc.URL); !allow {
		t.Fatalf("expected allow, got deny with status %d", w.Code)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/examples.auth.v1.AuthExampleService/Secret" {
		t.Errorf("path = %q, want the original request path", gotPath)
	}
	if got := gotHeaders.Get("Authorization"); got != "Bearer my-test-token" {
		t.Errorf("Authorization = %q, want it forwarded", got)
	}
	if got := gotHeaders.Get("Cookie"); got != "" {
		t.Errorf("Cookie = %q, want it NOT forwarded (verified live: Envoy's ext_authz doesn't send it either)", got)
	}
	if got := gotHeaders.Get("X-Custom-Test-Header"); got != "" {
		t.Errorf("X-Custom-Test-Header = %q, want it NOT forwarded", got)
	}
	// Go's own http.Client stamps a default User-Agent on any outbound
	// request that doesn't set one -- checking for "absent" would be
	// checking the wrong thing. The real invariant is that the *original*
	// client's User-Agent value doesn't leak through.
	if got := gotHeaders.Get("User-Agent"); got == "my-test-agent" {
		t.Errorf("User-Agent leaked the original request's value (%q); want it not forwarded", got)
	}
}

func TestAuthMiddleware_DenyRelaysStatusAndBody(t *testing.T) {
	authSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied by policy", http.StatusForbidden)
	}))
	defer authSvc.Close()

	req := httptest.NewRequest(http.MethodPost, "/some/path", nil)
	w := httptest.NewRecorder()

	mw := newAuthMiddleware()
	if allow := mw.check(w, req, authSvc.URL); allow {
		t.Fatal("expected deny")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAuthMiddleware_UnreachableAuthServiceDenies(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/some/path", nil)
	w := httptest.NewRecorder()

	mw := newAuthMiddleware()
	// Port 1 is reserved and nothing should ever be listening there.
	if allow := mw.check(w, req, "http://127.0.0.1:1"); allow {
		t.Fatal("expected deny when the auth service is unreachable (FailureModeAllow: false)")
	}
	if w.Code == http.StatusOK {
		t.Errorf("status = %d, want a non-200 failure response", w.Code)
	}
}
