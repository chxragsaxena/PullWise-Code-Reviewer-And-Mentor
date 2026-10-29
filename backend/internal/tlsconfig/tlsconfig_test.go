package tlsconfig

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func selfSigned(t *testing.T, dir string) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	certFile, keyFile = filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem")
	_ = os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	_ = os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
	return certFile, keyFile
}

func TestClientWithCAAndCertificate(t *testing.T) {
	cert, key := selfSigned(t, t.TempDir())
	cfg, err := Client(cert, key, cert)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RootCAs == nil || len(cfg.Certificates) != 1 {
		t.Fatalf("config = %+v, want a CA pool and one client certificate", cfg)
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %x, want TLS 1.3", cfg.MinVersion)
	}
}

func TestClientWithNothingUsesSystemRootsAndPresentsNoCertificate(t *testing.T) {
	cfg, err := Client("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RootCAs != nil || len(cfg.Certificates) != 0 {
		t.Fatalf("config = %+v, want system roots and no client certificate", cfg)
	}
}

func TestClientReportsBadFiles(t *testing.T) {
	dir := t.TempDir()
	cert, key := selfSigned(t, dir)
	junk := filepath.Join(dir, "junk.pem")
	_ = os.WriteFile(junk, []byte("not a certificate"), 0o600)

	for name, args := range map[string][3]string{
		"missing CA file":         {"", "", filepath.Join(dir, "nope.pem")},
		"CA file with no certs":   {"", "", junk},
		"certificate without key": {cert, filepath.Join(dir, "nope-key.pem"), ""},
		"key that does not match": {cert, junk, ""},
	} {
		if _, err := Client(args[0], args[1], args[2]); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	_ = key
}
