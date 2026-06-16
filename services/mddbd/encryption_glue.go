package main

import (
	"errors"
	"mddb/internal/encryption"
	"mddb/internal/storage"
)

// globalEncryptor is the process-wide singleton consulted by loadDoc
// to transparently decrypt documents at read time. Set once at
// startup from Server initialization; reads are lock-free because
// the pointer is written before any goroutine that reads from it.
var globalEncryptor *encryption.Encryptor

// SetGlobalEncryptor wires the process-wide encryptor used by the
// read path. Called exactly once at startup; tests pass nil to clear.
func SetGlobalEncryptor(e *encryption.Encryptor) { globalEncryptor = e }

// marshalAndEncrypt marshals a document and, when the given
// collection is opted into at-rest encryption AND a key is loaded,
// seals the resulting bytes before they reach the docs / rev buckets.
// When encryption is off the behaviour is identical to marshalDoc.
func marshalAndEncrypt(doc *storage.Doc, collection string) ([]byte, error) {
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
	if !encryption.IsEncrypted(data) {
		return data, nil
	}
	if globalEncryptor == nil {
		return nil, errors.New("encrypted payload but encryptor not initialized")
	}
	return globalEncryptor.Decrypt(data)
}
