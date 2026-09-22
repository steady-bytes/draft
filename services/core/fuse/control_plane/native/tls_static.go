package native

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"
)

// staticProvider is the operator-supplied CertificateProvider: a single
// cert/key pair loaded from disk, used for every domain regardless of SNI
// -- the common case for a wildcard cert from an existing CA, or a cert an
// upstream load balancer already manages. A future per-domain map is a
// natural extension (see the design doc's Certificate providers section)
// but isn't needed for v1: nothing in this codebase yet has more than one
// operator-supplied cert to choose between.
//
// Hot-reload is mtime-based, checked on every GetCertificate call, rather
// than an fsnotify watch -- avoids a new dependency for what's a cheap
// stat(2) on a path that changes rarely. "No restart on change" still
// holds: the next handshake after the file changes gets the new cert.
type staticProvider struct {
	certFile, keyFile string

	mu      sync.RWMutex
	cert    *tls.Certificate
	modTime time.Time

	// clientCAPool backs GetCACertPool for mTLS routes (Phase 6). Loaded
	// once at construction, not hot-reloaded -- a trusted-client-CA bundle
	// changes far less often than a server cert/key does in practice, and
	// mtime-polling it too would be the same mechanism copy-pasted for
	// comparatively little benefit. Revisit if that assumption is wrong.
	clientCAFile string
	clientCAPool *x509.CertPool

	changed chan string
}

// NewStaticProvider loads certFile/keyFile once at construction so a
// startup misconfiguration fails immediately rather than on the first
// handshake. clientCAFile is optional (pass "" if no route on this backend
// uses mTLS); GetCACertPool errors if called without one configured.
func NewStaticProvider(certFile, keyFile, clientCAFile string) (*staticProvider, error) {
	p := &staticProvider{certFile: certFile, keyFile: keyFile, clientCAFile: clientCAFile, changed: make(chan string, 8)}
	if err := p.reload(); err != nil {
		return nil, fmt.Errorf("loading operator-supplied cert/key: %w", err)
	}
	if clientCAFile != "" {
		pool, err := loadCACertPool(clientCAFile)
		if err != nil {
			return nil, fmt.Errorf("loading operator-supplied client CA bundle: %w", err)
		}
		p.clientCAPool = pool
	}
	return p, nil
}

// GetCACertPool returns the client-CA bundle configured via
// fuse.tls.operator.client_ca_file. name is not yet validated against
// anything -- this provider supports exactly one trust bundle.
func (p *staticProvider) GetCACertPool(name string) (*x509.CertPool, error) {
	if p.clientCAPool == nil {
		return nil, fmt.Errorf("mTLS route requires a trusted-CA pool, but fuse.tls.operator.client_ca_file is not configured")
	}
	return p.clientCAPool, nil
}

func loadCACertPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("no certificates found in %s", path)
	}
	return pool, nil
}

func (p *staticProvider) GetCertificate(domain string) (*tls.Certificate, error) {
	// Errors are intentionally swallowed here: serve the last-known-good
	// cert rather than failing a handshake over a transient read error
	// (eg. the operator is mid-rewrite of the file).
	p.reloadIfChanged()

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cert, nil
}

func (p *staticProvider) Changed() <-chan string { return p.changed }

// reloadIfChanged re-reads certFile/keyFile only if the cert file's mtime
// advanced since the last load.
func (p *staticProvider) reloadIfChanged() {
	info, err := os.Stat(p.certFile)
	if err != nil {
		return
	}
	p.mu.RLock()
	unchanged := !info.ModTime().After(p.modTime)
	p.mu.RUnlock()
	if unchanged {
		return
	}
	if err := p.reload(); err != nil {
		return
	}
	select {
	case p.changed <- p.certFile:
	default:
	}
}

func (p *staticProvider) reload() error {
	cert, err := tls.LoadX509KeyPair(p.certFile, p.keyFile)
	if err != nil {
		return err
	}
	info, err := os.Stat(p.certFile)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.cert = &cert
	p.modTime = info.ModTime()
	p.mu.Unlock()
	return nil
}
