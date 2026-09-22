package native

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestGenerateCA_IsSelfSignedAndValid(t *testing.T) {
	ca, key, certPEM, keyPEM, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA() error = %v", err)
	}
	if !ca.IsCA {
		t.Error("generated CA cert has IsCA = false")
	}
	if err := ca.CheckSignatureFrom(ca); err != nil {
		t.Errorf("CA cert is not self-signed: %v", err)
	}
	if key == nil {
		t.Fatal("generateCA() returned a nil key")
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatal("generateCA() returned empty PEM encoding")
	}
}

func TestGenerateCA_PEMRoundTrip(t *testing.T) {
	ca, _, certPEM, keyPEM, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA() error = %v", err)
	}

	// parseCAPEM is what loadCA feeds a concatenated cert+key PEM blob
	// through -- kvv1.Value has a single string field, so both blocks are
	// stored as one string and must decode back correctly regardless of
	// concatenation order.
	got, gotKey, err := parseCAPEM(append(append([]byte{}, certPEM...), keyPEM...))
	if err != nil {
		t.Fatalf("parseCAPEM() error = %v", err)
	}
	if got.SerialNumber.Cmp(ca.SerialNumber) != 0 {
		t.Errorf("round-tripped cert serial = %v, want %v", got.SerialNumber, ca.SerialNumber)
	}
	if gotKey == nil {
		t.Fatal("parseCAPEM() returned a nil key")
	}
}

func TestMintLeaf_SignedByCA(t *testing.T) {
	ca, key, _, _, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA() error = %v", err)
	}

	leaf, err := mintLeaf(ca, key, "orders.draft.localhost")
	if err != nil {
		t.Fatalf("mintLeaf() error = %v", err)
	}
	if leaf.Leaf == nil {
		t.Fatal("mintLeaf() did not set Leaf")
	}
	if leaf.Leaf.Subject.CommonName != "orders.draft.localhost" {
		t.Errorf("CommonName = %q, want the requested domain", leaf.Leaf.Subject.CommonName)
	}
	if len(leaf.Leaf.DNSNames) != 1 || leaf.Leaf.DNSNames[0] != "orders.draft.localhost" {
		t.Errorf("DNSNames = %v, want [orders.draft.localhost]", leaf.Leaf.DNSNames)
	}

	// A client that trusts the CA must be able to build a chain to this leaf.
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	if _, err := leaf.Leaf.Verify(x509.VerifyOptions{DNSName: "orders.draft.localhost", Roots: pool}); err != nil {
		t.Errorf("leaf cert does not verify against its issuing CA: %v", err)
	}
}

func TestSelfSignedProvider_CachesPerDomain(t *testing.T) {
	ca, key, _, _, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA() error = %v", err)
	}
	p := &selfSignedProvider{caCert: ca, caKey: key, leaves: map[string]*tls.Certificate{}, changed: make(chan string, 8)}

	c1, err := p.GetCertificate("orders.draft.localhost")
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}
	c2, err := p.GetCertificate("orders.draft.localhost")
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}
	if c1 != c2 {
		t.Error("expected the same cached *tls.Certificate for a repeated domain")
	}

	c3, err := p.GetCertificate("payments.draft.localhost")
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}
	if c3 == c1 {
		t.Error("expected a distinct certificate for a different domain")
	}
}

func TestExpiringSoon(t *testing.T) {
	ca, key, _, _, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA() error = %v", err)
	}
	fresh, err := mintLeaf(ca, key, "fresh.draft.localhost")
	if err != nil {
		t.Fatalf("mintLeaf() error = %v", err)
	}
	if expiringSoon(fresh) {
		t.Error("a freshly minted leaf should not be considered expiring soon")
	}

	almostExpired, err := mintLeaf(ca, key, "stale.draft.localhost")
	if err != nil {
		t.Fatalf("mintLeaf() error = %v", err)
	}
	almostExpired.Leaf.NotAfter = time.Now().Add(time.Minute)
	if !expiringSoon(almostExpired) {
		t.Error("a leaf one minute from expiry should be considered expiring soon")
	}
}
