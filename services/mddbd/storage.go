package main

import (
	"errors"

	json "github.com/goccy/go-json"
	"google.golang.org/protobuf/proto"
	pb "mddb/proto"
)

// Marshal document to protobuf bytes for storage with optional compression
func marshalDoc(doc *Doc) ([]byte, error) {
	protoDoc := docToProtoInternal(doc)
	data, err := proto.Marshal(protoDoc)
	if err != nil {
		return nil, err
	}

	// Compress if beneficial
	return compressDoc(data), nil
}

// Unmarshal document from protobuf bytes with decompression support.
// If data starts with the at-rest encryption magic prefix it is
// transparently decrypted before decompression.
func unmarshalDoc(data []byte) (*Doc, error) {
	if isEncrypted(data) {
		pt, err := maybeDecrypt(data)
		if err != nil {
			return nil, err
		}
		data = pt
	}
	// Decompress if needed
	decompressed, err := decompressDoc(data)
	if err != nil {
		return nil, err
	}

	protoDoc := &pb.Document{}
	if err := proto.Unmarshal(decompressed, protoDoc); err != nil {
		return nil, err
	}
	return protoToDoc(protoDoc), nil
}

// loadDoc auto-detects serialization format (JSON or protobuf+compression)
// and returns the deserialized Doc. JSON starts with '{' (0x7B), while
// protobuf+compression uses flag bytes 0, 1, or 2. When data starts
// with the AES-GCM magic prefix (MDDB_ENC_V1\x00), it is transparently
// decrypted first so the rest of the pipeline sees plaintext.
func loadDoc(data []byte) (*Doc, error) {
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
		var doc Doc
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		return &doc, nil
	}
	// Otherwise it's protobuf+compression format (flag byte 0, 1, or 2)
	return unmarshalDoc(data)
}

// Convert internal Doc to proto Document
func docToProtoInternal(doc *Doc) *pb.Document {
	protoMeta := make(map[string]*pb.MetaValues)
	for k, v := range doc.Meta {
		protoMeta[k] = &pb.MetaValues{Values: v}
	}

	return &pb.Document{
		Id:        doc.ID,
		Key:       doc.Key,
		Lang:      doc.Lang,
		Meta:      protoMeta,
		ContentMd: doc.ContentMD,
		AddedAt:   doc.AddedAt,
		UpdatedAt: doc.UpdatedAt,
		ExpiresAt: doc.ExpiresAt,
	}
}

// Convert proto Document to internal Doc
func protoToDoc(protoDoc *pb.Document) *Doc {
	meta := make(map[string][]string)
	for k, v := range protoDoc.Meta {
		meta[k] = v.Values
	}

	return &Doc{
		ID:        protoDoc.Id,
		Key:       protoDoc.Key,
		Lang:      protoDoc.Lang,
		Meta:      meta,
		ContentMD: protoDoc.ContentMd,
		AddedAt:   protoDoc.AddedAt,
		UpdatedAt: protoDoc.UpdatedAt,
		ExpiresAt: protoDoc.ExpiresAt,
	}
}
