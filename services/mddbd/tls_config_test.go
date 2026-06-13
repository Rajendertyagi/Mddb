package main

// Coverage for buildServerTLSConfig in tls_config.go.
//
// Each test case generates a fresh self-signed CA + server cert pair into a
// temp directory, then exercises one configuration permutation:
//
//   - disabled / missing fields → returns nil, nil
//   - bad cert path             → wraps the load error
//   - server cert + key only    → plain HTTPS, ClientAuth=NoClientCert
//   - server + client CA        → mTLS, default clientAuth=require
//   - server + client CA + "request" → mTLS, ClientAuth=VerifyClientCertIfGiven
//   - bad clientAuth value      → typed error
//   - missing client CA file    → wraps read error
//   - empty client CA bundle    → "no valid PEM" error

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

// makeTestKeyPair writes a fresh self-signed cert + key to dir/cert.pem and
// dir/key.pem and returns those paths. Used as both the server keypair and
// (when paired with another invocation) as a CA bundle for mTLS tests.
func makeTestKeyPair(t *testing.T, dir, name string) (certPath, keyPath string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPath = filepath.Join(dir, name+"-cert.pem")
	keyPath = filepath.Join(dir, name+"-key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}

func TestBuildServerTLSConfig_Disabled(t *testing.T) {
	got, err := buildServerTLSConfig(TLSConfig{Enabled: false})
	if err != nil {
		t.Fatalf("disabled should return nil,nil; got err=%v", err)
	}
	if got != nil {
		t.Errorf("disabled should return nil tls.Config, got %v", got)
	}
}

func TestBuildServerTLSConfig_MissingFields(t *testing.T) {
	cases := []TLSConfig{
		{Enabled: true},
		{Enabled: true, CertFile: "x.crt"},
		{Enabled: true, KeyFile: "x.key"},
	}
	for _, c := range cases {
		got, err := buildServerTLSConfig(c)
		if err != nil {
			t.Errorf("missing fields should be silent (returns nil,nil); got %v for %+v", err, c)
		}
		if got != nil {
			t.Errorf("expected nil tls.Config for %+v, got %v", c, got)
		}
	}
}

func TestBuildServerTLSConfig_BadCertPath(t *testing.T) {
	_, err := buildServerTLSConfig(TLSConfig{
		Enabled:  true,
		CertFile: "/no/such/cert.pem",
		KeyFile:  "/no/such/key.pem",
	})
	if err == nil {
		t.Fatal("expected error for missing cert/key files")
	}
}

func TestBuildServerTLSConfig_PlainHTTPS(t *testing.T) {
	dir := t.TempDir()
	cert, key := makeTestKeyPair(t, dir, "server")

	got, err := buildServerTLSConfig(TLSConfig{
		Enabled:  true,
		CertFile: cert,
		KeyFile:  key,
	})
	if err != nil {
		t.Fatalf("buildServerTLSConfig: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if got.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %x, want TLS 1.2 (%x)", got.MinVersion, tls.VersionTLS12)
	}
	if len(got.Certificates) != 1 {
		t.Errorf("Certificates len = %d, want 1", len(got.Certificates))
	}
	if got.ClientAuth != tls.NoClientCert {
		t.Errorf("ClientAuth = %v, want NoClientCert (no client CA configured)", got.ClientAuth)
	}
	if got.ClientCAs != nil {
		t.Error("ClientCAs should be nil when no clientCAFile is set")
	}
}

func TestBuildServerTLSConfig_MTLSRequireDefault(t *testing.T) {
	dir := t.TempDir()
	cert, key := makeTestKeyPair(t, dir, "server")
	caCert, _ := makeTestKeyPair(t, dir, "ca")

	got, err := buildServerTLSConfig(TLSConfig{
		Enabled:      true,
		CertFile:     cert,
		KeyFile:      key,
		ClientCAFile: caCert,
		// ClientAuth left empty → should default to "require"
	})
	if err != nil {
		t.Fatalf("buildServerTLSConfig: %v", err)
	}
	if got.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", got.ClientAuth)
	}
	if got.ClientCAs == nil {
		t.Error("ClientCAs should be populated when clientCAFile is set")
	}
}

func TestBuildServerTLSConfig_MTLSRequest(t *testing.T) {
	dir := t.TempDir()
	cert, key := makeTestKeyPair(t, dir, "server")
	caCert, _ := makeTestKeyPair(t, dir, "ca")

	got, err := buildServerTLSConfig(TLSConfig{
		Enabled:      true,
		CertFile:     cert,
		KeyFile:      key,
		ClientCAFile: caCert,
		ClientAuth:   "request",
	})
	if err != nil {
		t.Fatalf("buildServerTLSConfig: %v", err)
	}
	if got.ClientAuth != tls.VerifyClientCertIfGiven {
		t.Errorf("ClientAuth = %v, want VerifyClientCertIfGiven", got.ClientAuth)
	}
}

func TestBuildServerTLSConfig_MTLSExplicitRequire(t *testing.T) {
	dir := t.TempDir()
	cert, key := makeTestKeyPair(t, dir, "server")
	caCert, _ := makeTestKeyPair(t, dir, "ca")

	got, err := buildServerTLSConfig(TLSConfig{
		Enabled:      true,
		CertFile:     cert,
		KeyFile:      key,
		ClientCAFile: caCert,
		ClientAuth:   "require",
	})
	if err != nil {
		t.Fatalf("buildServerTLSConfig: %v", err)
	}
	if got.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", got.ClientAuth)
	}
}

func TestBuildServerTLSConfig_BadClientAuth(t *testing.T) {
	dir := t.TempDir()
	cert, key := makeTestKeyPair(t, dir, "server")
	caCert, _ := makeTestKeyPair(t, dir, "ca")

	_, err := buildServerTLSConfig(TLSConfig{
		Enabled:      true,
		CertFile:     cert,
		KeyFile:      key,
		ClientCAFile: caCert,
		ClientAuth:   "bogus",
	})
	if err == nil {
		t.Fatal("expected error for unknown clientAuth value")
	}
}

func TestBuildServerTLSConfig_MissingClientCAFile(t *testing.T) {
	dir := t.TempDir()
	cert, key := makeTestKeyPair(t, dir, "server")

	_, err := buildServerTLSConfig(TLSConfig{
		Enabled:      true,
		CertFile:     cert,
		KeyFile:      key,
		ClientCAFile: "/no/such/ca.crt",
	})
	if err == nil {
		t.Fatal("expected error for missing client CA file")
	}
}

func TestBuildServerTLSConfig_EmptyClientCABundle(t *testing.T) {
	dir := t.TempDir()
	cert, key := makeTestKeyPair(t, dir, "server")

	emptyCA := filepath.Join(dir, "empty.pem")
	if err := os.WriteFile(emptyCA, []byte("# this file has no certs\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := buildServerTLSConfig(TLSConfig{
		Enabled:      true,
		CertFile:     cert,
		KeyFile:      key,
		ClientCAFile: emptyCA,
	})
	if err == nil {
		t.Fatal("expected error for empty PEM bundle")
	}
}
