package native

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync/atomic"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// acmeProvider is the ACME CertificateProvider: certificates come from a
// real ACME directory (Let's Encrypt by default, or any RFC 8555-compatible
// CA via directoryURL) via golang.org/x/crypto/acme/autocert, which also
// handles renewal transparently through its own DirCache -- there is
// nothing for this package's own renewal/expiry logic to do, unlike
// selfSignedProvider.
type acmeProvider struct {
	m *autocert.Manager
}

// NewACMEProvider wires autocert's HostPolicy to the live route table
// (table.Load()) instead of a static domain list. This resolves the
// implementation plan's own flagged open question -- "rebuild on every
// Apply(), or accept a static config list" -- with a third option neither
// alternative considered: a HostPolicy function that consults the same
// live table matchRoute already reads, via selectHostScope. It's always
// exactly in sync with what's actually routed (no rebuild step to forget),
// and it reuses the existing host-matching logic instead of duplicating
// it into a second, parallel notion of "which domains are allowed."
// table may be nil only in tests that don't exercise HostPolicy.
func NewACMEProvider(directoryURL, cacheDir string, table *atomic.Pointer[RouteTable]) *acmeProvider {
	return &acmeProvider{m: &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache(cacheDir),
		Client: &acme.Client{DirectoryURL: directoryURL},
		HostPolicy: func(ctx context.Context, host string) error {
			if table == nil {
				return fmt.Errorf("no route table available to authorize ACME issuance for %q", host)
			}
			if len(selectHostScope(table.Load().routes, host)) == 0 {
				return fmt.Errorf("no route registered for host %q -- refusing ACME issuance", host)
			}
			return nil
		},
	}}
}

func (p *acmeProvider) GetCertificate(domain string) (*tls.Certificate, error) {
	return p.m.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})
}

// GetCACertPool: mTLS with an ACME-issued server certificate is not a
// combination anything in this codebase asks for -- ACME provisions
// server certs from a public CA, unrelated to validating client
// certificates -- so this provider declines rather than guessing at a
// trust source. A route with both mtls.enabled and an ACME-backed listener
// fails clearly here instead of silently picking an arbitrary pool.
func (p *acmeProvider) GetCACertPool(name string) (*x509.CertPool, error) {
	return nil, fmt.Errorf("the ACME certificate provider does not support mTLS client-CA pools")
}

// Changed: DirCache handles renewal entirely inside autocert, transparently
// to this package -- there is nothing to signal. Ranging over a nil
// channel (see Backend.logCertificateChanges) blocks forever, which is the
// correct behavior for "this provider never has anything to report", not
// a bug.
func (p *acmeProvider) Changed() <-chan string { return nil }
