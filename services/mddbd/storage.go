package main

import (
	"errors"

	"mddb/internal/compression"
	"mddb/internal/storage"
	pb "mddb/proto"

	json "github.com/goccy/go-json"
	"google.golang.org/protobuf/proto"
)

// Marshal document to protobuf bytes for storage with optional compression
func marshalDoc(doc *storage.Doc) ([]byte, error) {
	protoDoc := storage.DocToProto(doc)
	data, err := proto.Marshal(protoDoc)
	if err != nil {
		return nil, err
	}

	// Compress if beneficial
	return compression.CompressDoc(data), nil
}

// Unmarshal document from protobuf bytes with decompression support.
// If data starts with the at-rest encryption magic prefix it is
// transparently decrypted before decompression.
func unmarshalDoc(data []byte) (*storage.Doc, error) {
	if isEncrypted(data) {
		pt, err := maybeDecrypt(data)
		if err != nil {
			return nil, err
		}
		data = pt
	}
	// Decompress if needed
	decompressed, err := compression.DecompressDoc(data)
	if err != nil {
		return nil, err
	}

	protoDoc := &pb.Document{}
	if err := proto.Unmarshal(decompressed, protoDoc); err != nil {
		return nil, err
	}
	return storage.ProtoToDoc(protoDoc), nil
}

// loadDoc auto-detects serialization format (JSON or protobuf+compression)
// and returns the deserialized storage.Doc. JSON starts with '{' (0x7B), while
// protobuf+compression uses flag bytes 0, 1, or 2. When data starts
// with the AES-GCM magic prefix (MDDB_ENC_V1\x00), it is transparently
// decrypted first so the rest of the pipeline sees plaintext.
func loadDoc(data []byte) (*storage.Doc, error) {
	if len(data) == 0 {
		return nil, errors.New("empty document data")
	}
	if isEncrypted(data) {
		pt, err := maybeDecrypt(data)
		if err != nil {
			return nil, err
		}
		data = pt
	}
	// JSON always starts with '{' (0x7B = 123)
	if data[0] == '{' {
		var doc storage.Doc
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		return &doc, nil
	}
	// Otherwise it's protobuf+compression format (flag byte 0, 1, or 2)
	return unmarshalDoc(data)
}
