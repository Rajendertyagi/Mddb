package encryption

import (
	"encoding/json"
	"testing"
)

// encWithKeys builds an Encryptor from a primary key, key ID and the
// previous-keys JSON, all via the documented env vars.
func encWithKeys(t *testing.T, primary, id, prev string) *Encryptor {
	t.Helper()
	t.Setenv("MDDB_ENCRYPTION_KEY", primary)
	t.Setenv("MDDB_ENCRYPTION_KEY_ID", id)
	t.Setenv("MDDB_ENCRYPTION_KEYS_PREVIOUS", prev)
	e, err := NewEncryptor()
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	return e
}

func TestParseKeyIDRejectsOutOfRange(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", genKey(t))
	for _, bad := range []string{"0", "256", "-1", "abc"} {
		t.Setenv("MDDB_ENCRYPTION_KEY_ID", bad)
		if _, err := NewEncryptor(); err == nil {
			t.Errorf("KEY_ID=%q: expected error", bad)
		}
	}
}

func TestPrimaryAndPreviousKeyIDs(t *testing.T) {
	k1, k2 := genKey(t), genKey(t)
	prev, _ := json.Marshal([]previousKey{{ID: 1, Key: k1}})
	e := encWithKeys(t, k2, "2", string(prev))

	if e.PrimaryKeyID() != 2 {
		t.Errorf("PrimaryKeyID = %d, want 2", e.PrimaryKeyID())
	}
	ids := e.PreviousKeyIDs()
	if len(ids) != 1 || ids[0] != 1 {
		t.Errorf("PreviousKeyIDs = %v, want [1]", ids)
	}
}

func TestPreviousKeyDecryptAndUnknownLookup(t *testing.T) {
	k1, k2 := genKey(t), genKey(t)
	e1 := encWithKeys(t, k1, "1", "")
	e1.SetCollectionEnabled("c", true)
	old, err := e1.Encrypt([]byte("v1data"), "c")
	if err != nil {
		t.Fatal(err)
	}

	prev, _ := json.Marshal([]previousKey{{ID: 1, Key: k1}})
	e2 := encWithKeys(t, k2, "2", string(prev))
	if got, err := e2.Decrypt(old); err != nil || string(got) != "v1data" {
		t.Fatalf("previous-key decrypt: %q err=%v", got, err)
	}

	// An unknown keyID must error through lookupKey rather than return garbage.
	old[MagicLen] = 99
	if _, err := e2.Decrypt(old); err == nil {
		t.Fatal("expected unknown-keyID error")
	}
}

func TestDecryptV1WithPrimary(t *testing.T) {
	e := encWithKeys(t, genKey(t), "1", "")
	e.SetCollectionEnabled("c", true)
	v2, _ := e.Encrypt([]byte("legacy"), "c")

	// Forge a V1 payload: V1 magic followed by the V2 nonce+ciphertext
	// (drop the V2 keyID byte). The primary key decrypts it.
	v1 := append([]byte{}, MagicV1...)
	v1 = append(v1, v2[MagicLen+KeyIDLen:]...)
	if CiphertextVersion(v1) != 1 {
		t.Fatal("forged payload is not V1")
	}
	if pt, err := e.Decrypt(v1); err != nil || string(pt) != "legacy" {
		t.Fatalf("V1 decrypt: %q err=%v", pt, err)
	}
}

func TestIsEncryptedWithPrimaryBranches(t *testing.T) {
	k1, k2 := genKey(t), genKey(t)
	prev, _ := json.Marshal([]previousKey{{ID: 1, Key: k1}})
	e := encWithKeys(t, k2, "2", string(prev))
	e.SetCollectionEnabled("c", true)

	curr, _ := e.Encrypt([]byte("x"), "c")
	if !e.IsEncryptedWithPrimary(curr) {
		t.Error("current ciphertext should match the primary key")
	}
	curr[MagicLen] = 1 // previous keyID
	if e.IsEncryptedWithPrimary(curr) {
		t.Error("previous-key ciphertext must not match primary")
	}
	if e.IsEncryptedWithPrimary([]byte("plaintext")) {
		t.Error("plaintext must not match primary")
	}
}

func TestCiphertextKeyIDBranches(t *testing.T) {
	if _, ok := CiphertextKeyID([]byte("plaintext")); ok {
		t.Error("plaintext should carry no keyID")
	}
	e := encWithKeys(t, genKey(t), "5", "")
	e.SetCollectionEnabled("c", true)
	ct, _ := e.Encrypt([]byte("x"), "c")
	if id, ok := CiphertextKeyID(ct); !ok || id != 5 {
		t.Errorf("CiphertextKeyID = (%d,%v), want (5,true)", id, ok)
	}
}

func TestNewEncryptorPreviousKeyErrors(t *testing.T) {
	key := genKey(t)
	t.Setenv("MDDB_ENCRYPTION_KEY", key)
	t.Setenv("MDDB_ENCRYPTION_KEY_ID", "2")

	cases := map[string]string{
		"invalid json":  "{not json",
		"bad base64":    mustJSON([]previousKey{{ID: 1, Key: "!!!notbase64"}}),
		"collision":     mustJSON([]previousKey{{ID: 2, Key: key}}),
		"reserved zero": mustJSON([]previousKey{{ID: 0, Key: key}}),
	}
	for name, prev := range cases {
		t.Setenv("MDDB_ENCRYPTION_KEYS_PREVIOUS", prev)
		if _, err := NewEncryptor(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestDisabledEncryptorAccessors(t *testing.T) {
	t.Setenv("MDDB_ENCRYPTION_KEY", "")
	e, _ := NewEncryptor() // no key -> disabled
	if e.Enabled() {
		t.Fatal("expected disabled encryptor")
	}
	if e.PrimaryKeyID() != 0 {
		t.Errorf("disabled PrimaryKeyID = %d, want 0", e.PrimaryKeyID())
	}
	if ids := e.PreviousKeyIDs(); ids != nil {
		t.Errorf("disabled PreviousKeyIDs = %v, want nil", ids)
	}
	if _, err := e.decryptV1([]byte("anything")); err == nil {
		t.Error("decryptV1 on a disabled encryptor must error")
	}
}

func TestDecryptV1ErrorBranches(t *testing.T) {
	e := encWithKeys(t, genKey(t), "1", "")
	// Too short to hold magic + nonce.
	if _, err := e.decryptV1([]byte("short")); err == nil {
		t.Error("decryptV1 on a too-short payload must error")
	}
	// Correct length but garbage ciphertext -> AEAD open fails.
	bad := append(append([]byte{}, MagicV1...), make([]byte, NonceLen+16)...)
	if _, err := e.decryptV1(bad); err == nil {
		t.Error("decryptV1 on garbage ciphertext must error")
	}
}
