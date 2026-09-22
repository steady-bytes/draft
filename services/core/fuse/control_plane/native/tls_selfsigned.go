package native

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/anypb"
)

// selfSignedCAKVKey is the Blueprint KV key the local CA's cert+key persist
// under. Storing it in Blueprint KV (rather than a local file) means it
// moves with the cluster -- a dctl infra reset regenerating a new CA is
// consistent with everything else Blueprint owns resetting together. This
// resolves the design doc's own open question in favor of KV, per its
// stated lean.
const selfSignedCAKVKey = "fuse_local_ca"

// caLeafValidity is short deliberately: GetCertificate re-mints past this
// window with no separate renewal loop, so there's no reason to make it
// long -- see selfSignedProvider.GetCertificate.
const caLeafValidity = 7 * 24 * time.Hour

// caValidity is long: unlike a leaf, the CA itself isn't cheaply re-minted
// (every already-trusted leaf and every browser's "trust this CA" decision
// would need to happen again), so it's generated once and kept for years.
const caValidity = 10 * 365 * 24 * time.Hour

// selfSignedProvider is the local-dev CertificateProvider: Fuse generates
// its own root CA on first boot (persisted to Blueprint KV, see
// selfSignedCAKVKey) and mints short-lived leaf certificates per requested
// SNI on demand. Deliberately does not depend on the mkcert binary -- one
// point of this backend is fewer external binaries than the envoy backend
// needed, not a different one.
type selfSignedProvider struct {
	logger chassis.Logger
	caCert *x509.Certificate
	caKey  crypto.Signer

	mu     sync.Mutex
	leaves map[string]*tls.Certificate

	changed chan string
}

// NewSelfSignedProvider loads the CA from Blueprint KV if one already
// exists there, or generates and persists a new one if not. Prints a
// one-time trust instruction only when a new CA is generated, not on every
// boot.
func NewSelfSignedProvider(logger chassis.Logger) (*selfSignedProvider, error) {
	ctx := context.Background()
	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())

	ca, key, isNew, err := loadOrGenerateCA(ctx, client)
	if err != nil {
		return nil, err
	}
	if isNew {
		printTrustInstruction(logger, ca)
	}
	return &selfSignedProvider{
		logger:  logger,
		caCert:  ca,
		caKey:   key,
		leaves:  map[string]*tls.Certificate{},
		changed: make(chan string, 8),
	}, nil
}

func (p *selfSignedProvider) GetCertificate(domain string) (*tls.Certificate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if c, ok := p.leaves[domain]; ok && !expiringSoon(c) {
		return c, nil
	}

	leaf, err := mintLeaf(p.caCert, p.caKey, domain)
	if err != nil {
		return nil, fmt.Errorf("minting leaf certificate for %q: %w", domain, err)
	}
	_, existed := p.leaves[domain]
	p.leaves[domain] = leaf

	if existed {
		select {
		case p.changed <- domain:
		default: // non-blocking; Changed() is consumed for logging only, never required
		}
	}
	return leaf, nil
}

// GetCACertPool trusts client certificates signed by this provider's own
// local dev CA -- the same one used for server certs -- regardless of
// name. This provider manages exactly one CA, so there is nothing for name
// to select between yet; a developer testing mTLS locally mints a client
// cert from the same fuse-local-ca.pem printTrustInstruction already
// writes to disk.
func (p *selfSignedProvider) GetCACertPool(name string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	pool.AddCert(p.caCert)
	return pool, nil
}

func (p *selfSignedProvider) Changed() <-chan string { return p.changed }

// expiringSoon reports whether cert is within one validity-window's worth
// of buffer of its NotAfter -- re-minting well before actual expiry avoids
// a race where a handshake in flight sees a cert that expires mid-TLS
// setup. cert.Leaf is always populated here (mintLeaf sets it explicitly)
// so this never needs to re-parse cert.Certificate[0].
func expiringSoon(cert *tls.Certificate) bool {
	if cert.Leaf == nil {
		return true
	}
	return time.Now().After(cert.Leaf.NotAfter.Add(-caLeafValidity / 4))
}

// mintLeaf signs a new leaf certificate for domain with the CA's key.
// Serial numbers use crypto/rand (not time-based) -- two leaves minted in
// the same nanosecond, possible under concurrent GetCertificate calls for
// different domains, must not collide.
func mintLeaf(ca *x509.Certificate, caKey crypto.Signer, domain string) (*tls.Certificate, error) {
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    time.Now().Add(-time.Hour), // clock skew tolerance
		NotAfter:     time.Now().Add(caLeafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{der, ca.Raw}, // leaf + issuing CA, so a client trusting the CA can build the chain
		PrivateKey:  leafKey,
		Leaf:        leaf, // set explicitly -- avoids expiringSoon needing to re-parse on every call
	}, nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

// loadOrGenerateCA reads the CA from Blueprint KV (selfSignedCAKVKey); if
// absent, generates a new one and persists it before returning. isNew
// tells the caller whether to print the trust instruction.
func loadOrGenerateCA(ctx context.Context, client kvv1Connect.KeyValueServiceClient) (*x509.Certificate, crypto.Signer, bool, error) {
	if ca, key, err := loadCA(ctx, client); err == nil {
		return ca, key, false, nil
	}

	ca, key, certPEM, keyPEM, err := generateCA()
	if err != nil {
		return nil, nil, false, fmt.Errorf("generating local CA: %w", err)
	}

	val, err := anypb.New(&kvv1.Value{Data: string(certPEM) + string(keyPEM)})
	if err != nil {
		return nil, nil, false, err
	}
	if _, err := client.Set(ctx, connect.NewRequest(&kvv1.SetRequest{Key: selfSignedCAKVKey, Value: val})); err != nil {
		return nil, nil, false, fmt.Errorf("persisting local CA to blueprint: %w", err)
	}
	return ca, key, true, nil
}

func loadCA(ctx context.Context, client kvv1Connect.KeyValueServiceClient) (*x509.Certificate, crypto.Signer, error) {
	valAny, err := anypb.New(&kvv1.Value{})
	if err != nil {
		return nil, nil, err
	}
	resp, err := client.Get(ctx, connect.NewRequest(&kvv1.GetRequest{Key: selfSignedCAKVKey, Value: valAny}))
	if err != nil {
		return nil, nil, err // not found, or blueprint unreachable -- either way, caller generates fresh
	}
	value := &kvv1.Value{}
	if err := resp.Msg.GetValue().UnmarshalTo(value); err != nil {
		return nil, nil, err
	}
	return parseCAPEM([]byte(value.Data))
}

// parseCAPEM splits the concatenated cert+key PEM blob loadOrGenerateCA
// stores (kvv1.Value has a single string field -- no separate slot for a
// second value) back into its two blocks. PEM blocks are self-delimiting
// (BEGIN/END markers), so this is a plain decode loop, not a fragile
// string split on a chosen separator.
func parseCAPEM(data []byte) (*x509.Certificate, crypto.Signer, error) {
	var certDER, keyDER []byte
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch block.Type {
		case "CERTIFICATE":
			certDER = block.Bytes
		case "PRIVATE KEY":
			keyDER = block.Bytes
		}
	}
	if certDER == nil || keyDER == nil {
		return nil, nil, fmt.Errorf("stored CA value missing cert or key PEM block")
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(keyDER)
	if err != nil {
		return nil, nil, err
	}
	signer, ok := keyAny.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("stored CA private key does not implement crypto.Signer")
	}
	return cert, signer, nil
}

// generateCA creates a new root CA and returns it alongside its PEM
// encoding (for persistence) and parsed form (for immediate use).
func generateCA() (*x509.Certificate, crypto.Signer, []byte, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Fuse Local Development CA", Organization: []string{"Draft"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(caValidity),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return cert, key, certPEM, keyPEM, nil
}

// printTrustInstruction writes a copy of the new CA cert to disk and prints
// the one-time command to trust it -- the same UX mkcert -install gives a
// developer today, without the binary. Only called when a new CA was just
// generated, not on every boot with an existing one.
func printTrustInstruction(logger chassis.Logger, ca *x509.Certificate) {
	path := "fuse-local-ca.pem"
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
	if err := os.WriteFile(path, pemBytes, 0o644); err != nil {
		logger.WithError(err).Error("failed to write local CA to disk -- trust it manually from Blueprint KV (fuse_local_ca) instead")
		return
	}
	logger.WithField("path", path).Info(
		"generated a new local development CA for the native proxy backend -- " +
			"trust it once so browsers accept certificates it mints:\n" +
			"  macOS:  security add-trusted-cert -d -r trustRoot -k ~/Library/Keychains/login.keychain \"" + path + "\"\n" +
			"  Linux:  sudo cp \"" + path + "\" /usr/local/share/ca-certificates/fuse-local-ca.crt && sudo update-ca-certificates\n" +
			"  or import \"" + path + "\" into your browser's certificate trust store directly.",
	)
}
