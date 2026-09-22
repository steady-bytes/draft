package native

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestCertKeyPair(t *testing.T, dir, domain string) (certFile, keyFile string) {
	t.Helper()
	ca, caKey, _, _, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA() error = %v", err)
	}
	leaf, err := mintLeaf(ca, caKey, domain)
	if err != nil {
		t.Fatalf("mintLeaf() error = %v", err)
	}

	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Leaf.Raw})
	keyDER, err := x509.MarshalPKCS8PrivateKey(leaf.PrivateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}
	return certFile, keyFile
}

func TestStaticProvider_LoadsAndServes(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCertKeyPair(t, dir, "operator.draft.localhost")

	p, err := NewStaticProvider(certFile, keyFile, "")
	if err != nil {
		t.Fatalf("NewStaticProvider() error = %v", err)
	}
	cert, err := p.GetCertificate("anything") // static provider ignores SNI -- one cert for every domain
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}
	if cert == nil {
		t.Fatal("GetCertificate() returned a nil certificate")
	}
}

func TestStaticProvider_MissingFileErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewStaticProvider(filepath.Join(dir, "nope.pem"), filepath.Join(dir, "nope-key.pem"), ""); err == nil {
		t.Fatal("expected an error constructing a provider from missing files")
	}
}

func TestStaticProvider_HotReloadsOnFileChange(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCertKeyPair(t, dir, "v1.draft.localhost")

	p, err := NewStaticProvider(certFile, keyFile, "")
	if err != nil {
		t.Fatalf("NewStaticProvider() error = %v", err)
	}
	original, err := p.GetCertificate("anything")
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}

	// Overwrite with a different cert/key pair, forcing the mtime forward
	// in case the filesystem's mtime resolution is coarser than this test
	// runs in.
	_, _ = writeTestCertKeyPair(t, dir, "v2.draft.localhost")
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(certFile, future, future); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	updated, err := p.GetCertificate("anything")
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}
	if updated == original {
		t.Error("expected GetCertificate to pick up the file change without restarting the provider")
	}
	select {
	case <-p.Changed():
	default:
		t.Error("expected a signal on Changed() after a hot reload")
	}
}
