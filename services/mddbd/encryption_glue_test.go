package main

import (
	"crypto/rand"
	"encoding/base64"
	"mddb/internal/encryption"
	"mddb/internal/storage"
	"testing"
)

// genKey returns a base64-encoded 32-byte random key suitable for
// MDDB_ENCRYPTION_KEY. Shared by the encryption glue, rotation and handler tests.
func genKey(t *testing.T) string {
	t.Helper()
	k := make([]byte, encryption.KeyLen)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

// The following tests exercise the main-side glue (globalEncryptor,
// marshalAndEncrypt, maybeDecrypt) that bridges the crypto core in
// internal/encryption to the storage serialization (marshalDoc/loadDoc).

func TestLoadDocTransparentDecrypt(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	e, _ := encryption.NewEncryptor()
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)
	e.SetCollectionEnabled("secrets", true)

	doc := &storage.Doc{ID: "secrets|k|en", Key: "k", Lang: "en", ContentMD: "top-secret"}
	buf, err := marshalAndEncrypt(doc, "secrets")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !encryption.IsEncrypted(buf) {
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
	e, _ := encryption.NewEncryptor()
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)

	doc := &storage.Doc{ID: "blog|k|en", Key: "k", Lang: "en", ContentMD: "legacy"}
	buf, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if encryption.IsEncrypted(buf) {
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

func TestMaybeDecryptWithoutGlobalEncryptor(t *testing.T) {
	SetGlobalEncryptor(nil)
	// Plaintext passes through even when no global encryptor.
	if out, err := maybeDecrypt([]byte("plaintext")); err != nil || string(out) != "plaintext" {
		t.Fatalf("plaintext passthrough: %q %v", out, err)
	}
	// Ciphertext without a global encryptor must error.
	ct := append([]byte{}, encryption.MagicV2...)
	ct = append(ct, make([]byte, 40)...)
	if _, err := maybeDecrypt(ct); err == nil {
		t.Fatal("expected error when globalEncryptor nil")
	}
}

func TestMarshalAndEncryptNoGlobalEncryptor(t *testing.T) {
	SetGlobalEncryptor(nil)
	doc := &storage.Doc{ID: "x|k|en", Key: "k", Lang: "en"}
	buf, err := marshalAndEncrypt(doc, "x")
	if err != nil {
		t.Fatal(err)
	}
	if encryption.IsEncrypted(buf) {
		t.Fatal("no global encryptor: must passthrough")
	}
}

func TestMarshalAndEncryptFallbackWhenKeyMissing(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", "")
	e, _ := encryption.NewEncryptor()
	SetGlobalEncryptor(e)
	defer SetGlobalEncryptor(nil)

	doc := &storage.Doc{ID: "x|k|en", Key: "k", Lang: "en"}
	buf, err := marshalAndEncrypt(doc, "x")
	if err != nil {
		t.Fatal(err)
	}
	if encryption.IsEncrypted(buf) {
		t.Fatal("should fall back to plaintext when disabled")
	}
}
