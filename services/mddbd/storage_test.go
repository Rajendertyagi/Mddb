package main

import (
	"reflect"
	"strings"
	"testing"
)

// --- marshalDoc / unmarshalDoc round-trip ---

func TestMarshalUnmarshalDoc_Basic(t *testing.T) {
	doc := &Doc{
		ID:        "abc123",
		Key:       "homepage",
		Lang:      "en_GB",
		Meta:      map[string][]string{"tag": {"go", "db"}, "author": {"alice"}},
		ContentMD: "# Hello World",
		AddedAt:   1700000000,
		UpdatedAt: 1700001000,
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("marshalDoc returned empty data")
	}

	got, err := unmarshalDoc(data)
	if err != nil {
		t.Fatalf("unmarshalDoc failed: %v", err)
	}

	if got.ID != doc.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, doc.ID)
	}
	if got.Key != doc.Key {
		t.Errorf("Key mismatch: got %q, want %q", got.Key, doc.Key)
	}
	if got.Lang != doc.Lang {
		t.Errorf("Lang mismatch: got %q, want %q", got.Lang, doc.Lang)
	}
	if got.ContentMD != doc.ContentMD {
		t.Errorf("ContentMD mismatch: got %q, want %q", got.ContentMD, doc.ContentMD)
	}
	if got.AddedAt != doc.AddedAt {
		t.Errorf("AddedAt mismatch: got %d, want %d", got.AddedAt, doc.AddedAt)
	}
	if got.UpdatedAt != doc.UpdatedAt {
		t.Errorf("UpdatedAt mismatch: got %d, want %d", got.UpdatedAt, doc.UpdatedAt)
	}
	if !reflect.DeepEqual(got.Meta, doc.Meta) {
		t.Errorf("Meta mismatch: got %v, want %v", got.Meta, doc.Meta)
	}
}

func TestMarshalUnmarshalDoc_EmptyMeta(t *testing.T) {
	doc := &Doc{
		ID:        "x1",
		Key:       "empty",
		Lang:      "en",
		Meta:      map[string][]string{},
		ContentMD: "body",
		AddedAt:   100,
		UpdatedAt: 200,
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}

	got, err := unmarshalDoc(data)
	if err != nil {
		t.Fatalf("unmarshalDoc failed: %v", err)
	}

	if len(got.Meta) != 0 {
		t.Errorf("expected empty meta, got %v", got.Meta)
	}
}

func TestMarshalUnmarshalDoc_NilMeta(t *testing.T) {
	doc := &Doc{
		ID:        "x2",
		Key:       "nilmeta",
		Lang:      "de",
		Meta:      nil,
		ContentMD: "content",
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}

	got, err := unmarshalDoc(data)
	if err != nil {
		t.Fatalf("unmarshalDoc failed: %v", err)
	}

	// nil meta becomes empty map after proto round-trip
	if got.Meta == nil {
		// This is also acceptable
	} else if len(got.Meta) != 0 {
		t.Errorf("expected nil or empty meta, got %v", got.Meta)
	}
}

func TestMarshalUnmarshalDoc_ExpiresAt(t *testing.T) {
	doc := &Doc{
		ID:        "ttl1",
		Key:       "expiring",
		Lang:      "en",
		Meta:      map[string][]string{},
		ContentMD: "temporary",
		AddedAt:   1000,
		UpdatedAt: 2000,
		ExpiresAt: 9999,
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}

	got, err := unmarshalDoc(data)
	if err != nil {
		t.Fatalf("unmarshalDoc failed: %v", err)
	}

	if got.ExpiresAt != doc.ExpiresAt {
		t.Errorf("ExpiresAt mismatch: got %d, want %d", got.ExpiresAt, doc.ExpiresAt)
	}
}

func TestMarshalUnmarshalDoc_LargeContent(t *testing.T) {
	// Create a large document to trigger compression (>1KB)
	bigContent := strings.Repeat("This is a line of markdown content. ", 200)
	doc := &Doc{
		ID:        "big1",
		Key:       "bigpage",
		Lang:      "en",
		Meta:      map[string][]string{"section": {"docs"}, "tags": {"large", "test"}},
		ContentMD: bigContent,
		AddedAt:   5000,
		UpdatedAt: 6000,
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}

	got, err := unmarshalDoc(data)
	if err != nil {
		t.Fatalf("unmarshalDoc failed: %v", err)
	}

	if got.ContentMD != doc.ContentMD {
		t.Error("ContentMD mismatch after round-trip with large content")
	}
	if !reflect.DeepEqual(got.Meta, doc.Meta) {
		t.Errorf("Meta mismatch: got %v, want %v", got.Meta, doc.Meta)
	}
}

func TestMarshalUnmarshalDoc_VeryLargeContent(t *testing.T) {
	// >10KB triggers zstd compression path
	bigContent := strings.Repeat("abcdefghij", 2000)
	doc := &Doc{
		ID:        "huge1",
		Key:       "hugepage",
		Lang:      "en",
		Meta:      map[string][]string{},
		ContentMD: bigContent,
		AddedAt:   1,
		UpdatedAt: 2,
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}

	got, err := unmarshalDoc(data)
	if err != nil {
		t.Fatalf("unmarshalDoc failed: %v", err)
	}

	if got.ContentMD != doc.ContentMD {
		t.Error("ContentMD mismatch after round-trip with very large content")
	}
}

func TestUnmarshalDoc_EmptyData(t *testing.T) {
	_, err := unmarshalDoc([]byte{})
	if err == nil {
		t.Error("expected error for empty data, got nil")
	}
}

func TestUnmarshalDoc_InvalidData(t *testing.T) {
	// Invalid compressed data with a valid flag byte
	_, err := unmarshalDoc([]byte{flagUncompressed, 0xFF, 0xFF, 0xFF})
	if err == nil {
		t.Error("expected error for invalid protobuf data, got nil")
	}
}

func TestUnmarshalDoc_InvalidSnappyData(t *testing.T) {
	_, err := unmarshalDoc([]byte{flagSnappy, 0xFF, 0xFF, 0xFF, 0xFF})
	if err == nil {
		t.Error("expected error for invalid snappy data, got nil")
	}
}

func TestUnmarshalDoc_InvalidZstdData(t *testing.T) {
	_, err := unmarshalDoc([]byte{flagZstd, 0xFF, 0xFF, 0xFF, 0xFF})
	if err == nil {
		t.Error("expected error for invalid zstd data, got nil")
	}
}

// --- docToProtoInternal ---

func TestDocToProtoInternal_AllFields(t *testing.T) {
	doc := &Doc{
		ID:        "proto1",
		Key:       "prototest",
		Lang:      "fr",
		Meta:      map[string][]string{"cat": {"a", "b"}, "type": {"article"}},
		ContentMD: "# Proto",
		AddedAt:   111,
		UpdatedAt: 222,
		ExpiresAt: 333,
	}

	pb := docToProtoInternal(doc)

	if pb.Id != doc.ID {
		t.Errorf("Id mismatch: got %q, want %q", pb.Id, doc.ID)
	}
	if pb.Key != doc.Key {
		t.Errorf("Key mismatch: got %q, want %q", pb.Key, doc.Key)
	}
	if pb.Lang != doc.Lang {
		t.Errorf("Lang mismatch: got %q, want %q", pb.Lang, doc.Lang)
	}
	if pb.ContentMd != doc.ContentMD {
		t.Errorf("ContentMd mismatch: got %q, want %q", pb.ContentMd, doc.ContentMD)
	}
	if pb.AddedAt != doc.AddedAt {
		t.Errorf("AddedAt mismatch: got %d, want %d", pb.AddedAt, doc.AddedAt)
	}
	if pb.UpdatedAt != doc.UpdatedAt {
		t.Errorf("UpdatedAt mismatch: got %d, want %d", pb.UpdatedAt, doc.UpdatedAt)
	}
	if pb.ExpiresAt != doc.ExpiresAt {
		t.Errorf("ExpiresAt mismatch: got %d, want %d", pb.ExpiresAt, doc.ExpiresAt)
	}

	// Check meta conversion
	if len(pb.Meta) != 2 {
		t.Fatalf("Meta length mismatch: got %d, want 2", len(pb.Meta))
	}
	catVals := pb.Meta["cat"]
	if catVals == nil {
		t.Fatal("Meta key 'cat' not found")
		return
	}
	if !reflect.DeepEqual(catVals.Values, []string{"a", "b"}) {
		t.Errorf("Meta 'cat' values mismatch: got %v", catVals.Values)
	}
	typeVals := pb.Meta["type"]
	if typeVals == nil {
		t.Fatal("Meta key 'type' not found")
		return
	}
	if !reflect.DeepEqual(typeVals.Values, []string{"article"}) {
		t.Errorf("Meta 'type' values mismatch: got %v", typeVals.Values)
	}
}

func TestDocToProtoInternal_NilMeta(t *testing.T) {
	doc := &Doc{
		ID:   "nm1",
		Meta: nil,
	}

	pb := docToProtoInternal(doc)
	if len(pb.Meta) != 0 {
		t.Errorf("expected empty meta, got %v", pb.Meta)
	}
}

// --- protoToDoc ---

func TestProtoToDoc_AllFields(t *testing.T) {
	doc := &Doc{
		ID:        "rt1",
		Key:       "roundtrip",
		Lang:      "es",
		Meta:      map[string][]string{"x": {"1", "2"}, "y": {"3"}},
		ContentMD: "content",
		AddedAt:   100,
		UpdatedAt: 200,
		ExpiresAt: 300,
	}

	protoDoc := docToProtoInternal(doc)
	got := protoToDoc(protoDoc)

	if !reflect.DeepEqual(got, doc) {
		t.Errorf("round-trip mismatch:\ngot:  %+v\nwant: %+v", got, doc)
	}
}

func TestProtoToDoc_EmptyProto(t *testing.T) {
	protoDoc := docToProtoInternal(&Doc{Meta: map[string][]string{}})
	got := protoToDoc(protoDoc)

	if got.ID != "" {
		t.Errorf("expected empty ID, got %q", got.ID)
	}
	if len(got.Meta) != 0 {
		t.Errorf("expected empty meta, got %v", got.Meta)
	}
}

// --- Compression behavior verification ---

func TestMarshalDoc_SmallDocNotCompressed(t *testing.T) {
	doc := &Doc{
		ID:        "s1",
		Key:       "small",
		Lang:      "en",
		Meta:      map[string][]string{},
		ContentMD: "tiny",
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}

	// Small data should have uncompressed flag
	if len(data) == 0 {
		t.Fatal("empty marshaled data")
	}
	if data[0] != flagUncompressed {
		t.Errorf("expected uncompressed flag (0x%02x), got 0x%02x", flagUncompressed, data[0])
	}
}

func TestMarshalDoc_MultipleMetaValues(t *testing.T) {
	doc := &Doc{
		ID:   "mv1",
		Key:  "multimeta",
		Lang: "en",
		Meta: map[string][]string{
			"tags":       {"go", "database", "markdown", "test"},
			"categories": {"tech", "software"},
			"author":     {"alice"},
		},
		ContentMD: "# Multi Meta",
		AddedAt:   1000,
		UpdatedAt: 2000,
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}

	got, err := unmarshalDoc(data)
	if err != nil {
		t.Fatalf("unmarshalDoc failed: %v", err)
	}

	if !reflect.DeepEqual(got.Meta, doc.Meta) {
		t.Errorf("Meta mismatch after round-trip:\ngot:  %v\nwant: %v", got.Meta, doc.Meta)
	}
}

func TestMarshalUnmarshalDoc_UnicodeContent(t *testing.T) {
	doc := &Doc{
		ID:        "uni1",
		Key:       "unicode",
		Lang:      "ja",
		Meta:      map[string][]string{"title": {"日本語テスト"}},
		ContentMD: "# こんにちは世界\n\nThis is a test with emoji: 🎉 and CJK: 中文",
		AddedAt:   1000,
		UpdatedAt: 2000,
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}

	got, err := unmarshalDoc(data)
	if err != nil {
		t.Fatalf("unmarshalDoc failed: %v", err)
	}

	if got.ContentMD != doc.ContentMD {
		t.Errorf("Unicode ContentMD mismatch: got %q, want %q", got.ContentMD, doc.ContentMD)
	}
	if !reflect.DeepEqual(got.Meta, doc.Meta) {
		t.Errorf("Unicode Meta mismatch: got %v, want %v", got.Meta, doc.Meta)
	}
}

func TestMarshalUnmarshalDoc_EmptyStrings(t *testing.T) {
	doc := &Doc{
		ID:        "",
		Key:       "",
		Lang:      "",
		Meta:      map[string][]string{"": {""}},
		ContentMD: "",
	}

	data, err := marshalDoc(doc)
	if err != nil {
		t.Fatalf("marshalDoc failed: %v", err)
	}

	got, err := unmarshalDoc(data)
	if err != nil {
		t.Fatalf("unmarshalDoc failed: %v", err)
	}

	if got.ID != "" || got.Key != "" || got.Lang != "" || got.ContentMD != "" {
		t.Errorf("expected empty strings, got ID=%q Key=%q Lang=%q ContentMD=%q",
			got.ID, got.Key, got.Lang, got.ContentMD)
	}
}
