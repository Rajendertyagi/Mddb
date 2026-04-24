package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sync"
)

// encryptionMagic is the fixed 12-byte prefix that identifies a
// ciphertext blob produced by this module. It lets loadDoc detect
// encrypted payloads at read-time without carrying per-key metadata
// in a separate bucket — the format is self-describing.
var encryptionMagic = []byte("MDDB_ENC_V1\x00")

const (
	encryptionMagicLen = 12 // len(encryptionMagic)
	encryptionNonceLen = 12 // AES-GCM standard nonce length
	encryptionKeyLen   = 32 // AES-256
)

// Encryptor performs opt-in AES-256-GCM value-level encryption for
// documents stored in specific collections. Activation requires
// BOTH a process-wide key (MDDB_ENCRYPTION_KEY, base64) and a
// per-collection flag (CollectionConfig.Encrypted=true) — the
// encryptor is a no-op otherwise so existing deployments and
// collections that hold non-sensitive data pay zero cost.
//
// Wire format: magic(12) | nonce(12) | ciphertext+tag
//
// The encryptor never mutates its inputs; callers pass marshaled
// (and already compressed) document bytes and receive the sealed
// payload ready for Put.
type Encryptor struct {
	enabled bool
	gcm     cipher.AEAD
	mu      sync.RWMutex
	// collectionEnabled mirrors CollectionConfig.Encrypted so the
	// hot path does not need to hit the CollectionManager store on
	// every marshal.
	collectionEnabled map[string]bool
}

// NewEncryptor reads MDDB_ENCRYPTION_KEY and returns an encryptor.
// When the env var is empty or invalid the returned encryptor is
// a no-op and an explanatory error is returned for the caller to
// surface at startup — but the server can still boot so dev
// workflows aren't broken by a missing key.
func NewEncryptor() (*Encryptor, error) {
	e := &Encryptor{collectionEnabled: make(map[string]bool)}
	raw := os.Getenv("MDDB_ENCRYPTION_KEY")
	if raw == "" {
		return e, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return e, fmt.Errorf("MDDB_ENCRYPTION_KEY: base64 decode: %w", err)
	}
	if len(key) != encryptionKeyLen {
		return e, fmt.Errorf("MDDB_ENCRYPTION_KEY: want %d bytes (AES-256), got %d", encryptionKeyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return e, fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return e, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	e.gcm = gcm
	e.enabled = true
	return e, nil
}

// Enabled reports whether a usable key is loaded.
func (e *Encryptor) Enabled() bool { return e != nil && e.enabled }

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
// Never mutates the input slice.
func (e *Encryptor) Encrypt(plaintext []byte, collection string) ([]byte, error) {
	if !e.Enabled() || !e.CollectionEnabled(collection) {
		return plaintext, nil
	}
	return e.sealRaw(plaintext)
}

// EncryptAlways seals plaintext unconditionally (when a key is
// loaded). Used by callers that have already decided to encrypt —
// e.g. integration tests and future offline-migration tooling.
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
	// magic | nonce | ciphertext+tag
	out := make([]byte, 0, encryptionMagicLen+encryptionNonceLen+len(plaintext)+e.gcm.Overhead())
	out = append(out, encryptionMagic...)
	out = append(out, nonce...)
	out = e.gcm.Seal(out, nonce, plaintext, nil)
	return out, nil
}

// Decrypt reverses Encrypt when data starts with the magic prefix,
// otherwise returns data as-is so legacy plaintext documents keep
// reading after a collection is opted in.
func (e *Encryptor) Decrypt(data []byte) ([]byte, error) {
	if !isEncrypted(data) {
		return data, nil
	}
	if !e.Enabled() {
		return nil, errors.New("encrypted payload but MDDB_ENCRYPTION_KEY not set")
	}
	if len(data) < encryptionMagicLen+encryptionNonceLen {
		return nil, errors.New("encrypted payload too short")
	}
	nonce := data[encryptionMagicLen : encryptionMagicLen+encryptionNonceLen]
	ct := data[encryptionMagicLen+encryptionNonceLen:]
	pt, err := e.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return pt, nil
}

// isEncrypted reports whether data is a payload produced by
// Encryptor.sealRaw — i.e. starts with the magic prefix.
func isEncrypted(data []byte) bool {
	if len(data) < encryptionMagicLen {
		return false
	}
	return bytes.Equal(data[:encryptionMagicLen], encryptionMagic)
}

// globalEncryptor is the process-wide singleton consulted by loadDoc
// to transparently decrypt documents at read time. Set once at
// startup from Server initialization; reads are lock-free because
// the pointer is written before any goroutine that reads from it.
var globalEncryptor *Encryptor

// SetGlobalEncryptor wires the process-wide encryptor used by the
// read path. Called exactly once at startup.
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
// when the magic prefix is present and passing plaintext through
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
