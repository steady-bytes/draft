package native

import (
	"crypto/tls"
	"crypto/x509"
)

// CertificateProvider abstracts where the native backend's TLS certificates
// come from -- local self-signed (selfSignedProvider), operator-supplied
// (staticProvider), or ACME (Phase 7, not built yet). See
// docs/website/content/docs/architecture/fuse-native-proxy.md's TLS
// section.
type CertificateProvider interface {
	// GetCertificate returns the certificate to present for domain (the
	// incoming TLS ClientHello's SNI). Called fresh on every handshake --
	// see backend.go's GetConfigForClient wiring -- so a provider-side
	// renewal is picked up on the very next connection with no explicit
	// propagation step required.
	GetCertificate(domain string) (*tls.Certificate, error)

	// GetCACertPool returns the trusted-CA pool a client certificate must
	// chain to for an mTLS-enabled route (see MTLSPolicy.trusted_ca_secret_name).
	// name is not yet validated against anything by either implementation
	// as of Phase 6 -- neither supports more than one distinct trust
	// bundle -- see each provider's own doc comment.
	GetCACertPool(name string) (*x509.CertPool, error)

	// Changed fires (best-effort; sends are non-blocking) when a
	// certificate for a previously-served domain has been renewed or
	// replaced. Consumed today only for logging -- no listener restart is
	// needed, since GetCertificate is re-consulted on every handshake
	// regardless.
	Changed() <-chan string
}

const (
	// TLSModeConfigKey selects the native backend's certificate source.
	TLSModeConfigKey = "fuse.tls.mode"

	TLSModeOff        = "off"
	TLSModeSelfSigned = "self-signed"
	TLSModeOperator   = "operator"
	TLSModeACME       = "acme"

	// acmeDefaultDirectoryURL is Let's Encrypt's production directory,
	// used when fuse.tls.acme.directory_url is unset. A non-default value
	// is what makes any RFC 8555-compatible CA usable, not only Let's
	// Encrypt -- see NewACMEProvider.
	acmeDefaultDirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
)
