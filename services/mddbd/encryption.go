package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Wire formats
//
//	V1: MDDB_ENC_V1\x00 (12B) | nonce (12B)               | ciphertext+tag
//	V2: MDDB_ENC_V2\x00 (12B) | keyID (1B) | nonce (12B)  | ciphertext+tag
//
// V1 is the legacy format from 2.9.15. V2 (2.9.16+) prefixes the
// payload with an 8-bit key identifier so the encryptor can hold the
// primary key plus any number of read-only previous keys; rotation
// flips the primary, leaves the previous in place to keep historical
// documents readable, and an admin job optionally re-encrypts every
// document to converge the database on the new primary.
//
// V1 ciphertexts are decrypted with the primary key — the assumption
// is that pre-2.9.16 deployments only ever had one key.
var (
	encryptionMagicV1 = []byte("MDDB_ENC_V1\x00")
	encryptionMagicV2 = []byte("MDDB_ENC_V2\x00")
)

const (
	encryptionMagicLen = 12 // len(encryptionMagicV1) == len(encryptionMagicV2)
	encryptionNonceLen = 12 // AES-GCM standard nonce length
	encryptionKeyLen   = 32 // AES-256
	encryptionKeyIDLen = 1  // V2 keyID is a single byte (1..255)
)

// previousKey is one entry of MDDB_ENCRYPTION_KEYS_PREVIOUS — a
// read-only key used to decrypt documents written before a rotation.
type previousKey struct {
	ID  byte   `json:"id"`
	Key string `json:"key"` // base64 of 32 random bytes
}

// Encryptor performs opt-in AES-256-GCM value-level encryption for
// documents stored in specific collections. Activation requires
// BOTH a process-wide primary key (MDDB_ENCRYPTION_KEY, base64) and
// a per-collection flag (CollectionConfig.Encrypted=true) — the
// encryptor is a no-op otherwise so existing deployments and
// collections that hold non-sensitive data pay zero cost.
//
// Rotation: set MDDB_ENCRYPTION_KEY to the new key, give it a fresh
// MDDB_ENCRYPTION_KEY_ID, and list every superseded key in
// MDDB_ENCRYPTION_KEYS_PREVIOUS so historical documents stay readable.
type Encryptor struct {
	enabled   bool
	primaryID byte
	primary   cipher.AEAD
	previous  map[byte]cipher.AEAD // keyID → AEAD; read-only
	mu        sync.RWMutex
	// collectionEnabled mirrors CollectionConfig.Encrypted so the
	// hot path does not need to hit the CollectionManager store on
	// every marshal.
	collectionEnabled map[string]bool
}

// NewEncryptor reads MDDB_ENCRYPTION_KEY (and optional rotation env
// vars) and returns an encryptor. When the primary env var is empty
// or invalid the returned encryptor is a no-op and an explanatory
// error is returned for the caller to surface at startup — but the
// server can still boot so dev workflows aren't broken by a missing
// key.
func NewEncryptor() (*Encryptor, error) {
	e := &Encryptor{
		previous:          make(map[byte]cipher.AEAD),
		collectionEnabled: make(map[string]bool),
	}
	raw := os.Getenv("MDDB_ENCRYPTION_KEY")
	if raw == "" {
		return e, nil
	}
	primary, err := buildAEAD(raw)
	if err != nil {
		return e, fmt.Errorf("MDDB_ENCRYPTION_KEY: %w", err)
	}

	// Default keyID = 1 so V2 ciphertexts never carry an ambiguous 0
	// (which would otherwise collide with the implicit "treat V1 as
	// primary" rule used during decrypt).
	keyID, err := parseKeyID(os.Getenv("MDDB_ENCRYPTION_KEY_ID"), 1)
	if err != nil {
		return e, fmt.Errorf("MDDB_ENCRYPTION_KEY_ID: %w", err)
	}
	e.primaryID = keyID
	e.primary = primary
	e.enabled = true

	if rawPrev := strings.TrimSpace(os.Getenv("MDDB_ENCRYPTION_KEYS_PREVIOUS")); rawPrev != "" {
		var entries []previousKey
		if err := json.Unmarshal([]byte(rawPrev), &entries); err != nil {
			return e, fmt.Errorf("MDDB_ENCRYPTION_KEYS_PREVIOUS: parse: %w", err)
		}
		for _, p := range entries {
			if p.ID == 0 {
				return e, errors.New("MDDB_ENCRYPTION_KEYS_PREVIOUS: keyID 0 is reserved")
			}
			if p.ID == keyID {
				return e, fmt.Errorf("MDDB_ENCRYPTION_KEYS_PREVIOUS: keyID %d collides with primary", p.ID)
			}
			aead, err := buildAEAD(p.Key)
			if err != nil {
				return e, fmt.Errorf("MDDB_ENCRYPTION_KEYS_PREVIOUS[id=%d]: %w", p.ID, err)
			}
			e.previous[p.ID] = aead
		}
	}
	return e, nil
}

// buildAEAD validates a base64-encoded 32-byte key and returns an
// AES-256-GCM AEAD. Helper shared by primary and previous key parsing.
func buildAEAD(rawBase64 string) (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	if len(key) != encryptionKeyLen {
		return nil, fmt.Errorf("want %d bytes (AES-256), got %d", encryptionKeyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	return gcm, nil
}

// parseKeyID converts an env value into a 1..255 byte. Empty input
// returns the supplied default so a single-key deployment never has
// to set MDDB_ENCRYPTION_KEY_ID by hand.
func parseKeyID(raw string, def byte) (byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("not an integer: %w", err)
	}
	if n < 1 || n > 255 {
		return 0, fmt.Errorf("must be 1..255, got %d", n)
	}
	return byte(n), nil
}

// Enabled reports whether a usable primary key is loaded.
func (e *Encryptor) Enabled() bool { return e != nil && e.enabled }

// PrimaryKeyID returns the active key identifier; 0 when disabled.
func (e *Encryptor) PrimaryKeyID() byte {
	if !e.Enabled() {
		return 0
	}
	return e.primaryID
}

// PreviousKeyIDs returns the read-only key IDs currently loaded.
// Order is unspecified; intended for status reporting.
func (e *Encryptor) PreviousKeyIDs() []byte {
	if !e.Enabled() {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]byte, 0, len(e.previous))
	for id := range e.previous {
		out = append(out, id)
	}
	return out
}

// SetCollectionEnabled updates the per-collection opt-in flag. Safe
// for concurrent use. Called from CollectionManager.Set() and once
// at startup during LoadAll().
func (e *Encryptor) SetCollectionEnabled(collection string, on bool) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if on {
		if e.collectionEnabled == nil {
			e.collectionEnabled = make(map[string]bool)
		}
		e.collectionEnabled[collection] = true
	} else {
		delete(e.collectionEnabled, collection)
	}
}

// CollectionEnabled reports whether the given collection is opted
// into encryption.
func (e *Encryptor) CollectionEnabled(collection string) bool {
	if e == nil {
		return false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.collectionEnabled[collection]
}

// Encrypt seals plaintext for storage when the encryptor is active
// AND the collection is opted in. Otherwise returns plaintext as-is.
// Never mutates the input slice. Always uses the primary key.
func (e *Encryptor) Encrypt(plaintext []byte, collection string) ([]byte, error) {
	if !e.Enabled() || !e.CollectionEnabled(collection) {
		return plaintext, nil
	}
	return e.sealRaw(plaintext)
}

// EncryptAlways seals plaintext unconditionally with the primary key
// (when one is loaded). Used by callers that have already decided to
// encrypt — e.g. integration tests and the rotation worker.
func (e *Encryptor) EncryptAlways(plaintext []byte) ([]byte, error) {
	if !e.Enabled() {
		return nil, errors.New("encryption not configured")
	}
	return e.sealRaw(plaintext)
}

func (e *Encryptor) sealRaw(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, encryptionNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("rand nonce: %w", err)
	}
	// V2: magic | keyID | nonce | ciphertext+tag
	out := make([]byte, 0, encryptionMagicLen+encryptionKeyIDLen+encryptionNonceLen+len(plaintext)+e.primary.Overhead())
	out = append(out, encryptionMagicV2...)
	out = append(out, e.primaryID)
	out = append(out, nonce...)
	out = e.primary.Seal(out, nonce, plaintext, nil)
	return out, nil
}

// Decrypt reverses Encrypt when data starts with a recognised magic
// prefix, otherwise returns data as-is so legacy plaintext documents
// keep reading after a collection is opted in.
func (e *Encryptor) Decrypt(data []byte) ([]byte, error) {
	switch ciphertextVersion(data) {
	case 0:
		return data, nil // plaintext passthrough
	case 1:
		return e.decryptV1(data)
	case 2:
		return e.decryptV2(data)
	default:
		return nil, errors.New("unknown encryption magic")
	}
}

// IsEncryptedWithPrimary reports whether the ciphertext is sealed
// with the current primary key — used by the rotation worker to skip
// already-converged documents.
func (e *Encryptor) IsEncryptedWithPrimary(data []byte) bool {
	if !e.Enabled() {
		return false
	}
	id, ok := ciphertextKeyID(data)
	if !ok {
		return false
	}
	return id == e.primaryID
}

func (e *Encryptor) decryptV1(data []byte) ([]byte, error) {
	if !e.Enabled() {
		return nil, errors.New("encrypted payload but MDDB_ENCRYPTION_KEY not set")
	}
	if len(data) < encryptionMagicLen+encryptionNonceLen {
		return nil, errors.New("encrypted payload too short")
	}
	nonce := data[encryptionMagicLen : encryptionMagicLen+encryptionNonceLen]
	ct := data[encryptionMagicLen+encryptionNonceLen:]
	pt, err := e.primary.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt v1: %w", err)
	}
	return pt, nil
}

func (e *Encryptor) decryptV2(data []byte) ([]byte, error) {
	if !e.Enabled() {
		return nil, errors.New("encrypted payload but MDDB_ENCRYPTION_KEY not set")
	}
	if len(data) < encryptionMagicLen+encryptionKeyIDLen+encryptionNonceLen {
		return nil, errors.New("encrypted payload too short")
	}
	keyID := data[encryptionMagicLen]
	nonce := data[encryptionMagicLen+encryptionKeyIDLen : encryptionMagicLen+encryptionKeyIDLen+encryptionNonceLen]
	ct := data[encryptionMagicLen+encryptionKeyIDLen+encryptionNonceLen:]

	aead, err := e.lookupKey(keyID)
	if err != nil {
		return nil, err
	}
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt v2 (keyID=%d): %w", keyID, err)
	}
	return pt, nil
}

// lookupKey resolves a V2 keyID to its AEAD instance, checking primary
// then previous keys.
func (e *Encryptor) lookupKey(id byte) (cipher.AEAD, error) {
	if id == e.primaryID {
		return e.primary, nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if k, ok := e.previous[id]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown encryption keyID %d", id)
}

// ciphertextVersion returns 1 for V1, 2 for V2, 0 for plaintext, -1
// for "looks like an MDDB envelope but the version is unrecognised".
func ciphertextVersion(data []byte) int {
	if len(data) < encryptionMagicLen {
		return 0
	}
	if bytes.Equal(data[:encryptionMagicLen], encryptionMagicV2) {
		return 2
	}
	if bytes.Equal(data[:encryptionMagicLen], encryptionMagicV1) {
		return 1
	}
	// Any other "MDDB_ENC_V*\x00" lookalike is treated as plaintext —
	// callers that want strictness use ciphertextKeyID().
	return 0
}

// ciphertextKeyID returns the V2 keyID. Returns (id, true) for V2,
// (primaryID-marker, true) for V1 only when caller wraps in an
// Encryptor method that knows the primary; here just (0, false) for
// V1 because the helper is version-agnostic.
func ciphertextKeyID(data []byte) (byte, bool) {
	if ciphertextVersion(data) != 2 {
		return 0, false
	}
	if len(data) < encryptionMagicLen+encryptionKeyIDLen {
		return 0, false
	}
	return data[encryptionMagicLen], true
}

// isEncrypted reports whether data is a payload produced by sealRaw —
// either the legacy V1 format or the current V2.
func isEncrypted(data []byte) bool {
	v := ciphertextVersion(data)
	return v == 1 || v == 2
}

// globalEncryptor is the process-wide singleton consulted by loadDoc
// to transparently decrypt documents at read time. Set once at
// startup from Server initialization; reads are lock-free because
// the pointer is written before any goroutine that reads from it.
var globalEncryptor *Encryptor

// SetGlobalEncryptor wires the process-wide encryptor used by the
// read path. Called exactly once at startup; tests pass nil to clear.
func SetGlobalEncryptor(e *Encryptor) { globalEncryptor = e }

// marshalAndEncrypt marshals a document and, when the given
// collection is opted into at-rest encryption AND a key is loaded,
// seals the resulting bytes before they reach the docs / rev buckets.
// When encryption is off the behaviour is identical to marshalDoc.
func marshalAndEncrypt(doc *Doc, collection string) ([]byte, error) {
	buf, err := marshalDoc(doc)
	if err != nil {
		return nil, err
	}
	if globalEncryptor == nil {
		return buf, nil
	}
	return globalEncryptor.Encrypt(buf, collection)
}

// maybeDecrypt returns plaintext for data — transparently decrypting
// when a magic prefix is present and passing plaintext through
// otherwise. Safe to call when no encryptor is configured; such a
// call only errors if the caller hands in ciphertext without a key.
func maybeDecrypt(data []byte) ([]byte, error) {
	if !isEncrypted(data) {
		return data, nil
	}
	if globalEncryptor == nil {
		return nil, errors.New("encrypted payload but encryptor not initialized")
	}
	return globalEncryptor.Decrypt(data)
}
