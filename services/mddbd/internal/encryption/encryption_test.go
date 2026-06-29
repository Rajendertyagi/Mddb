package encryption

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

// genKey returns a base64-encoded 32-byte random key suitable for
// MDDB_ENCRYPTION_KEY.
func genKey(t *testing.T) string {
	t.Helper()
	k := make([]byte, KeyLen)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

func TestNewEncryptorDisabledByDefault(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", "")
	e, err := NewEncryptor()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if e.Enabled() {
		t.Fatal("expected disabled when env unset")
	}
	// Encrypt passes plaintext through.
	out, err := e.Encrypt([]byte("hello"), "blog")
	if err != nil || !bytes.Equal(out, []byte("hello")) {
		t.Fatalf("want passthrough, got %q, err=%v", out, err)
	}
}

func TestNewEncryptorBadBase64(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", "!!!not-base64!!!")
	e, err := NewEncryptor()
	if err == nil {
		t.Fatal("expected base64 error")
	}
	if e.Enabled() {
		t.Fatal("encryptor must be disabled when key is invalid")
	}
}

func TestNewEncryptorWrongKeyLength(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("short")))
	_, err := NewEncryptor()
	if err == nil {
		t.Fatal("expected key-length error")
	}
	if !strings.Contains(err.Error(), "32") {
		t.Errorf("err message should mention required length: %v", err)
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, err := NewEncryptor()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	e.SetCollectionEnabled("secrets", true)

	plaintext := []byte("the quick brown fox")
	ct, err := e.Encrypt(plaintext, "secrets")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(ct, plaintext) {
		t.Fatal("ciphertext must not equal plaintext")
	}
	if !IsEncrypted(ct) {
		t.Fatal("ciphertext missing magic prefix")
	}
	pt, err := e.Decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Fatalf("roundtrip mismatch: got %q", pt)
	}
}

func TestEncryptCollectionNotOptedIn(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := NewEncryptor()
	// "public" is not opted in.
	ct, err := e.Encrypt([]byte("hello"), "public")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ct, []byte("hello")) {
		t.Fatalf("want passthrough, got %q", ct)
	}
}

func TestDecryptPassthroughOnPlaintext(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := NewEncryptor()
	out, err := e.Decrypt([]byte("plaintext doc"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, []byte("plaintext doc")) {
		t.Fatalf("want passthrough, got %q", out)
	}
}

func TestDecryptErrorsWhenKeyMissing(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := NewEncryptor()
	e.SetCollectionEnabled("x", true)
	ct, _ := e.Encrypt([]byte("v"), "x")

	// Now build a fresh encryptor with no key and hand it the blob.
	t.Setenv("MDDB_ENCRYPTION_KEY", "")
	e2, _ := NewEncryptor()
	if _, err := e2.Decrypt(ct); err == nil {
		t.Fatal("expected error when decrypting without a key")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	a, _ := NewEncryptor()
	a.SetCollectionEnabled("x", true)
	ct, _ := a.Encrypt([]byte("v"), "x")

	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	b, _ := NewEncryptor()
	if _, err := b.Decrypt(ct); err == nil {
		t.Fatal("expected GCM auth tag failure")
	}
}

func TestDecryptShortPayload(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := NewEncryptor()
	// Magic prefix present but no room for nonce + tag.
	short := append([]byte{}, MagicV2...)
	if _, err := e.Decrypt(short); err == nil {
		t.Fatal("expected error on short ciphertext")
	}
}

func TestCollectionEnabledToggle(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := NewEncryptor()
	e.SetCollectionEnabled("x", true)
	if !e.CollectionEnabled("x") {
		t.Fatal("expected enabled")
	}
	e.SetCollectionEnabled("x", false)
	if e.CollectionEnabled("x") {
		t.Fatal("expected disabled after unset")
	}
}

func TestNilEncryptorSafe(t *testing.T) {
	var e *Encryptor
	if e.Enabled() {
		t.Fatal("nil must report disabled")
	}
	if e.CollectionEnabled("x") {
		t.Fatal("nil must report disabled")
	}
	e.SetCollectionEnabled("x", true) // must not panic
}

func TestIsEncryptedPrefix(t *testing.T) {
	if IsEncrypted([]byte("short")) {
		t.Fatal("short should not match")
	}
	if !IsEncrypted(append([]byte(MagicV2), 0, 1, 2)) {
		t.Fatal("V2 magic prefix should match")
	}
	if !IsEncrypted(append([]byte(MagicV1), 0, 1, 2)) {
		t.Fatal("V1 magic prefix should still match (backward compat)")
	}
}

func TestEncryptAlwaysWithoutKey(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", "")
	e, _ := NewEncryptor()
	if _, err := e.EncryptAlways([]byte("x")); err == nil {
		t.Fatal("expected error without key")
	}
}

func TestEncryptAlwaysRoundtrip(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := NewEncryptor()
	ct, err := e.EncryptAlways([]byte("v"))
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(ct) {
		t.Fatal("expected ciphertext")
	}
	pt, err := e.Decrypt(ct)
	if err != nil || string(pt) != "v" {
		t.Fatalf("roundtrip failed: %q err=%v", pt, err)
	}
}
