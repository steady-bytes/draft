package native

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
)

// mintTestClientCert signs a client-auth certificate with the given CA --
// deliberately separate from mintLeaf, which is Fuse's own server-cert
// minting function (ExtKeyUsageServerAuth only) and is not usable for a
// client certificate: Go's TLS stack correctly rejects a cert presented as
// a client credential unless it carries ExtKeyUsageClientAuth, something
// mintLeaf's real product code has no reason to set. This exists purely to
// simulate what an external tool (openssl, a real client-cert workflow)
// would produce when issuing a client cert from Fuse's exported
// fuse-local-ca.pem -- Fuse itself has no need to mint client certs, only
// to validate them (GetCACertPool).
func mintTestClientCert(ca *x509.Certificate, caKey crypto.Signer, cn string) (*tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}

// TestTLSHandshake_SelfSignedProviderEndToEnd exercises the actual wiring
// a real listener uses (tlsConfigForProvider, the same function
// buildTLSConfig calls) against a real TCP connection and a real TLS
// handshake -- not just the certificate-generation logic the other tests
// in this package already cover. This is the one piece of Phase 5 that
// isn't replicating existing Envoy behavior (unlike Phases 2-4), so it
// gets its own live-handshake test rather than relying on unit coverage
// of the crypto helpers alone.
func TestTLSHandshake_SelfSignedProviderEndToEnd(t *testing.T) {
	ca, key, _, _, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA() error = %v", err)
	}
	provider := &selfSignedProvider{caCert: ca, caKey: key, leaves: map[string]*tls.Certificate{}, changed: make(chan string, 8)}

	server := &http.Server{
		Handler:   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") }),
		TLSConfig: tlsConfigForProvider(provider, nil),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	go server.ServeTLS(ln, "", "") //nolint:errcheck
	defer server.Close()

	pool := x509.NewCertPool()
	pool.AddCert(ca)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				ServerName: "orders.draft.localhost", // drives SNI -> GetConfigForClient -> GetCertificate
			},
		},
	}

	resp, err := client.Get("https://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatalf("client request failed (handshake or connection error): %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	if string(body) != "ok\n" {
		t.Errorf("body = %q, want %q", body, "ok\n")
	}

	// TLS 1.3's certificate message is encrypted, so http.Response doesn't
	// expose the served cert directly -- confirming the request succeeded
	// against a client that only trusts `ca`, with ServerName set to the
	// SNI value routed through GetCertificate, is itself proof the right
	// leaf was minted and presented; the earlier TestMintLeaf_SignedByCA
	// already checks the leaf's own fields independently.
}

// TestTLSHandshake_UntrustedClientRejected confirms a client that does NOT
// trust the self-signed CA fails the handshake, the same way a browser
// that hasn't imported fuse-local-ca.pem would.
func TestTLSHandshake_UntrustedClientRejected(t *testing.T) {
	ca, key, _, _, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA() error = %v", err)
	}
	provider := &selfSignedProvider{caCert: ca, caKey: key, leaves: map[string]*tls.Certificate{}, changed: make(chan string, 8)}

	server := &http.Server{
		Handler:   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		TLSConfig: tlsConfigForProvider(provider, nil),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	go server.ServeTLS(ln, "", "") //nolint:errcheck
	defer server.Close()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{ServerName: "orders.draft.localhost"}, // no RootCAs -- uses the system pool, which won't trust this CA
		},
	}
	if _, err := client.Get("https://" + ln.Addr().String() + "/"); err == nil {
		t.Fatal("expected the handshake to fail for a client that doesn't trust the local CA")
	}
}

// TestTLSHandshake_MTLSRequiresClientCert exercises Phase 6's real wiring
// end to end: a route with mtls.enabled routes through routeForMTLS into
// ClientAuth/ClientCAs, using the same selfSignedProvider.GetCACertPool
// that trusts its own local dev CA -- so a client certificate minted from
// that same CA is accepted, and a request with none, or one from an
// unrelated CA, is rejected at the handshake.
func TestTLSHandshake_MTLSRequiresClientCert(t *testing.T) {
	ca, key, _, _, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA() error = %v", err)
	}
	provider := &selfSignedProvider{caCert: ca, caKey: key, leaves: map[string]*tls.Certificate{}, changed: make(chan string, 8)}

	var table atomic.Pointer[RouteTable]
	table.Store(&RouteTable{routes: []*ntv1.Route{
		{
			Name:  "secure",
			Match: &ntv1.RouteMatch{Host: "secure.draft.localhost", Prefix: "/"},
			Mtls:  &ntv1.MTLSPolicy{Enabled: true},
		},
	}})

	server := &http.Server{
		Handler:   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") }),
		TLSConfig: tlsConfigForProvider(provider, &table),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	go server.ServeTLS(ln, "", "") //nolint:errcheck
	defer server.Close()

	serverPool := x509.NewCertPool()
	serverPool.AddCert(ca)

	t.Run("no client cert is rejected", func(t *testing.T) {
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs: serverPool, ServerName: "secure.draft.localhost",
		}}}
		if _, err := client.Get("https://" + ln.Addr().String() + "/"); err == nil {
			t.Fatal("expected the handshake to fail with no client certificate presented")
		}
	})

	t.Run("client cert from an unrelated CA is rejected", func(t *testing.T) {
		otherCA, otherKey, _, _, err := generateCA()
		if err != nil {
			t.Fatalf("generateCA() error = %v", err)
		}
		clientLeaf, err := mintTestClientCert(otherCA, otherKey, "someone")
		if err != nil {
			t.Fatalf("mintTestClientCert() error = %v", err)
		}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs: serverPool, ServerName: "secure.draft.localhost",
			Certificates: []tls.Certificate{*clientLeaf},
		}}}
		if _, err := client.Get("https://" + ln.Addr().String() + "/"); err == nil {
			t.Fatal("expected the handshake to fail with a client cert from a CA the server doesn't trust")
		}
	})

	t.Run("client cert from the trusted CA is accepted", func(t *testing.T) {
		clientLeaf, err := mintTestClientCert(ca, key, "trusted-client")
		if err != nil {
			t.Fatalf("mintTestClientCert() error = %v", err)
		}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs: serverPool, ServerName: "secure.draft.localhost",
			Certificates: []tls.Certificate{*clientLeaf},
		}}}
		resp, err := client.Get("https://" + ln.Addr().String() + "/")
		if err != nil {
			t.Fatalf("expected the handshake to succeed with a client cert from the trusted CA: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading response body: %v", err)
		}
		if string(body) != "ok\n" {
			t.Errorf("body = %q, want %q", body, "ok\n")
		}
	})
}
