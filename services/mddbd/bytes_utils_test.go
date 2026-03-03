package main

import (
	"bytes"
	"testing"
)

// --- BytesSplit ---

func TestBytesSplit_Basic(t *testing.T) {
	data := []byte("doc|blog|post1")
	parts := BytesSplit(data, '|')

	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	if string(parts[0]) != "doc" {
		t.Errorf("parts[0] = %q, want %q", parts[0], "doc")
	}
	if string(parts[1]) != "blog" {
		t.Errorf("parts[1] = %q, want %q", parts[1], "blog")
	}
	if string(parts[2]) != "post1" {
		t.Errorf("parts[2] = %q, want %q", parts[2], "post1")
	}
}

func TestBytesSplit_Empty(t *testing.T) {
	parts := BytesSplit(nil, '|')
	if parts != nil {
		t.Errorf("BytesSplit(nil) = %v, want nil", parts)
	}

	parts = BytesSplit([]byte{}, '|')
	if parts != nil {
		t.Errorf("BytesSplit([]) = %v, want nil", parts)
	}
}

func TestBytesSplit_NoSeparator(t *testing.T) {
	parts := BytesSplit([]byte("nosep"), '|')
	if len(parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(parts))
	}
	if string(parts[0]) != "nosep" {
		t.Errorf("parts[0] = %q, want %q", parts[0], "nosep")
	}
}

func TestBytesSplit_TrailingSeparator(t *testing.T) {
	parts := BytesSplit([]byte("a|b|"), '|')
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	if string(parts[2]) != "" {
		t.Errorf("parts[2] = %q, want empty", parts[2])
	}
}

func TestBytesSplit_LeadingSeparator(t *testing.T) {
	parts := BytesSplit([]byte("|a|b"), '|')
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	if string(parts[0]) != "" {
		t.Errorf("parts[0] = %q, want empty", parts[0])
	}
}

func TestBytesSplit_ConsecutiveSeparators(t *testing.T) {
	parts := BytesSplit([]byte("a||b"), '|')
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	if string(parts[1]) != "" {
		t.Errorf("parts[1] = %q, want empty", parts[1])
	}
}

// --- BytesHasPrefix ---

func TestBytesHasPrefix_True(t *testing.T) {
	if !BytesHasPrefix([]byte("doc|blog|x"), []byte("doc|")) {
		t.Error("should have prefix 'doc|'")
	}
}

func TestBytesHasPrefix_False(t *testing.T) {
	if BytesHasPrefix([]byte("doc|blog"), []byte("rev|")) {
		t.Error("should not have prefix 'rev|'")
	}
}

func TestBytesHasPrefix_EmptyPrefix(t *testing.T) {
	if !BytesHasPrefix([]byte("anything"), []byte{}) {
		t.Error("any string has empty prefix")
	}
}

func TestBytesHasPrefix_LongerPrefix(t *testing.T) {
	if BytesHasPrefix([]byte("ab"), []byte("abcdef")) {
		t.Error("shorter data cannot have longer prefix")
	}
}

func TestBytesHasPrefix_ExactMatch(t *testing.T) {
	if !BytesHasPrefix([]byte("exact"), []byte("exact")) {
		t.Error("exact match should return true")
	}
}

// --- BytesIndexByte ---

func TestBytesIndexByte_Found(t *testing.T) {
	idx := BytesIndexByte([]byte("hello|world"), '|')
	if idx != 5 {
		t.Errorf("index = %d, want 5", idx)
	}
}

func TestBytesIndexByte_NotFound(t *testing.T) {
	idx := BytesIndexByte([]byte("hello"), '|')
	if idx != -1 {
		t.Errorf("index = %d, want -1", idx)
	}
}

func TestBytesIndexByte_First(t *testing.T) {
	idx := BytesIndexByte([]byte("|hello"), '|')
	if idx != 0 {
		t.Errorf("index = %d, want 0", idx)
	}
}

func TestBytesIndexByte_Empty(t *testing.T) {
	idx := BytesIndexByte([]byte{}, '|')
	if idx != -1 {
		t.Errorf("index = %d, want -1", idx)
	}
}

// --- BytesLastIndexByte ---

func TestBytesLastIndexByte_Found(t *testing.T) {
	idx := BytesLastIndexByte([]byte("a|b|c"), '|')
	if idx != 3 {
		t.Errorf("index = %d, want 3", idx)
	}
}

func TestBytesLastIndexByte_NotFound(t *testing.T) {
	idx := BytesLastIndexByte([]byte("abc"), '|')
	if idx != -1 {
		t.Errorf("index = %d, want -1", idx)
	}
}

func TestBytesLastIndexByte_Last(t *testing.T) {
	idx := BytesLastIndexByte([]byte("hello|"), '|')
	if idx != 5 {
		t.Errorf("index = %d, want 5", idx)
	}
}

func TestBytesLastIndexByte_Empty(t *testing.T) {
	idx := BytesLastIndexByte([]byte{}, '|')
	if idx != -1 {
		t.Errorf("index = %d, want -1", idx)
	}
}

// --- ExtractPart ---

func TestExtractPart_Basic(t *testing.T) {
	data := []byte("doc|blog|post1")

	tests := []struct {
		index int
		want  string
	}{
		{0, "doc"},
		{1, "blog"},
		{2, "post1"},
	}

	for _, tt := range tests {
		got := ExtractPart(data, tt.index)
		if string(got) != tt.want {
			t.Errorf("ExtractPart(%d) = %q, want %q", tt.index, got, tt.want)
		}
	}
}

func TestExtractPart_OutOfRange(t *testing.T) {
	data := []byte("a|b")
	if got := ExtractPart(data, 5); got != nil {
		t.Errorf("ExtractPart(5) = %q, want nil", got)
	}
}

func TestExtractPart_Empty(t *testing.T) {
	if got := ExtractPart(nil, 0); got != nil {
		t.Errorf("ExtractPart(nil, 0) = %v, want nil", got)
	}
	if got := ExtractPart([]byte{}, 0); got != nil {
		t.Errorf("ExtractPart([], 0) = %v, want nil", got)
	}
}

func TestExtractPart_SinglePart(t *testing.T) {
	got := ExtractPart([]byte("solo"), 0)
	if string(got) != "solo" {
		t.Errorf("ExtractPart = %q, want %q", got, "solo")
	}
}

// --- FormatTimestamp ---

func TestFormatTimestamp_Basic(t *testing.T) {
	buf := make([]byte, 20)
	result := FormatTimestamp(1700000000, buf)

	expected := "00000000001700000000"
	if string(result) != expected {
		t.Errorf("FormatTimestamp = %q, want %q", result, expected)
	}
}

func TestFormatTimestamp_Zero(t *testing.T) {
	buf := make([]byte, 20)
	result := FormatTimestamp(0, buf)

	expected := "00000000000000000000"
	if string(result) != expected {
		t.Errorf("FormatTimestamp(0) = %q, want %q", result, expected)
	}
}

func TestFormatTimestamp_SmallBuffer(t *testing.T) {
	// Buffer too small: should allocate new
	buf := make([]byte, 5)
	result := FormatTimestamp(42, buf)

	if len(result) != 20 {
		t.Errorf("result len = %d, want 20", len(result))
	}
	expected := "00000000000000000042"
	if string(result) != expected {
		t.Errorf("FormatTimestamp = %q, want %q", result, expected)
	}
}

func TestFormatTimestamp_LargeValue(t *testing.T) {
	buf := make([]byte, 20)
	result := FormatTimestamp(99999999999999999, buf)

	if len(result) != 20 {
		t.Errorf("result len = %d, want 20", len(result))
	}
	// Should be zero-padded to 20 digits
	expected := "00099999999999999999"
	if string(result) != expected {
		t.Errorf("FormatTimestamp = %q, want %q", result, expected)
	}
}

// --- AppendBytes ---

func TestAppendBytes_Basic(t *testing.T) {
	dst := []byte("hello")
	result := AppendBytes(dst, []byte(" "), []byte("world"))
	if string(result) != "hello world" {
		t.Errorf("AppendBytes = %q, want %q", result, "hello world")
	}
}

func TestAppendBytes_Empty(t *testing.T) {
	result := AppendBytes(nil, []byte("a"), []byte("b"))
	if string(result) != "ab" {
		t.Errorf("AppendBytes = %q, want %q", result, "ab")
	}
}

func TestAppendBytes_NoParts(t *testing.T) {
	dst := []byte("unchanged")
	result := AppendBytes(dst)
	if string(result) != "unchanged" {
		t.Errorf("AppendBytes = %q, want %q", result, "unchanged")
	}
}

func TestAppendBytes_EmptyParts(t *testing.T) {
	result := AppendBytes(nil, []byte{}, []byte("data"), []byte{})
	if string(result) != "data" {
		t.Errorf("AppendBytes = %q, want %q", result, "data")
	}
}

// --- BytesToLower ---

func TestBytesToLower(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello world"},
		{"ALLCAPS", "allcaps"},
		{"already lower", "already lower"},
		{"MiXeD123", "mixed123"},
		{"", ""},
		{"ABC", "abc"},
	}

	for _, tt := range tests {
		b := []byte(tt.input)
		BytesToLower(b)
		if string(b) != tt.want {
			t.Errorf("BytesToLower(%q) = %q, want %q", tt.input, b, tt.want)
		}
	}
}

// --- CompareBytes ---

func TestCompareBytes(t *testing.T) {
	tests := []struct {
		a, b []byte
		want int
	}{
		{[]byte("abc"), []byte("abc"), 0},
		{[]byte("abc"), []byte("abd"), -1},
		{[]byte("abd"), []byte("abc"), 1},
		{[]byte("ab"), []byte("abc"), -1},
		{[]byte("abc"), []byte("ab"), 1},
		{nil, nil, 0},
		{[]byte{}, []byte{}, 0},
		{nil, []byte("a"), -1},
		{[]byte("a"), nil, 1},
	}

	for _, tt := range tests {
		got := CompareBytes(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("CompareBytes(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// --- CopyBytes ---

func TestCopyBytes_Basic(t *testing.T) {
	src := []byte("hello")
	dst := CopyBytes(src)

	if !bytes.Equal(dst, src) {
		t.Errorf("CopyBytes = %q, want %q", dst, src)
	}

	// Verify it's a true copy (modifying dst does not affect src)
	dst[0] = 'X'
	if src[0] == 'X' {
		t.Error("CopyBytes did not create independent copy")
	}
}

func TestCopyBytes_Nil(t *testing.T) {
	if got := CopyBytes(nil); got != nil {
		t.Errorf("CopyBytes(nil) = %v, want nil", got)
	}
}

func TestCopyBytes_Empty(t *testing.T) {
	got := CopyBytes([]byte{})
	if got == nil {
		t.Error("CopyBytes(empty) should not return nil")
	}
	if len(got) != 0 {
		t.Errorf("CopyBytes(empty) len = %d, want 0", len(got))
	}
}
