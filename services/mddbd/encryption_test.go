package main

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
	k := make([]byte, encryptionKeyLen)
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
	if !isEncrypted(ct) {
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
	short := append([]byte{}, encryptionMagicV2...)
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
	if isEncrypted([]byte("short")) {
		t.Fatal("short should not match")
	}
	if !isEncrypted(append([]byte(encryptionMagicV2), 0, 1, 2)) {
		t.Fatal("V2 magic prefix should match")
	}
	if !isEncrypted(append([]byte(encryptionMagicV1), 0, 1, 2)) {
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

func TestLoadDocTransparentDecrypt(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := NewEncryptor()
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)
	e.SetCollectionEnabled("secrets", true)

	doc := &Doc{ID: "secrets|k|en", Key: "k", Lang: "en", ContentMD: "top-secret"}
	buf, err := marshalAndEncrypt(doc, "secrets")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !isEncrypted(buf) {
		t.Fatal("buf should be ciphertext")
	}
	got, err := loadDoc(buf)
	if err != nil {
		t.Fatalf("loadDoc: %v", err)
	}
	if got.ContentMD != "top-secret" {
		t.Fatalf("roundtrip failed: %+v", got)
	}
}

func TestLoadDocBackwardCompatPlaintext(t *testing.T) {
	// Set a key but the doc was saved before encryption — no magic prefix.
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := NewEncryptor()
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)

	doc := &Doc{ID: "blog|k|en", Key: "k", Lang: "en", ContentMD: "legacy"}
	buf, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if isEncrypted(buf) {
		t.Fatal("plain marshal must not carry the magic prefix")
	}
	got, err := loadDoc(buf)
	if err != nil {
		t.Fatalf("loadDoc: %v", err)
	}
	if got.ContentMD != "legacy" {
		t.Fatalf("got %+v", got)
	}
}

func TestEncryptAlwaysRoundtrip(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := NewEncryptor()
	ct, err := e.EncryptAlways([]byte("v"))
	if err != nil {
		t.Fatal(err)
	}
	if !isEncrypted(ct) {
		t.Fatal("expected ciphertext")
	}
	pt, err := e.Decrypt(ct)
	if err != nil || string(pt) != "v" {
		t.Fatalf("roundtrip failed: %q err=%v", pt, err)
	}
}

func TestMaybeDecryptWithoutGlobalEncryptor(t *testing.T) {
	SetGlobalEncryptor(nil)
	// Plaintext passes through even when no global encryptor.
	if out, err := maybeDecrypt([]byte("plaintext")); err != nil || string(out) != "plaintext" {
		t.Fatalf("plaintext passthrough: %q %v", out, err)
	}
	// Ciphertext without a global encryptor must error.
	ct := append([]byte{}, encryptionMagicV2...)
	ct = append(ct, make([]byte, 40)...)
	if _, err := maybeDecrypt(ct); err == nil {
		t.Fatal("expected error when globalEncryptor nil")
	}
}

func TestMarshalAndEncryptNoGlobalEncryptor(t *testing.T) {
	SetGlobalEncryptor(nil)
	doc := &Doc{ID: "x|k|en", Key: "k", Lang: "en"}
	buf, err := marshalAndEncrypt(doc, "x")
	if err != nil {
		t.Fatal(err)
	}
	if isEncrypted(buf) {
		t.Fatal("no global encryptor: must passthrough")
	}
}

func TestMarshalAndEncryptFallbackWhenKeyMissing(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", "")
	e, _ := NewEncryptor()
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)

	doc := &Doc{ID: "x|k|en", Key: "k", Lang: "en"}
	buf, err := marshalAndEncrypt(doc, "x")
	if err != nil {
		t.Fatal(err)
	}
	if isEncrypted(buf) {
		t.Fatal("should fall back to plaintext when disabled")
	}
}
