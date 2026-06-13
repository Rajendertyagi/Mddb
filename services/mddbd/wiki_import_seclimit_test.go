package main

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

// SEC-006: cappedReader bounds how many decompressed bytes a wiki import may
// consume. (compress/bzip2 is decompress-only in the stdlib, so the bomb is
// simulated by an oversized reader feeding the same XML-decoder path the bz2
// stream uses.)

func TestCappedReader_UnderLimitPassesThrough(t *testing.T) {
	r := &cappedReader{r: strings.NewReader("hello world"), limit: 100}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("unexpected error under limit: %v", err)
	}
	if string(b) != "hello world" {
		t.Errorf("got %q, want %q", b, "hello world")
	}
}

func TestCappedReader_OverLimitErrors(t *testing.T) {
	r := &cappedReader{r: strings.NewReader(strings.Repeat("a", 1000)), limit: 100}
	_, err := io.ReadAll(r)
	if !errors.Is(err, errWikiDecompressedLimit) {
		t.Errorf("over limit: got %v, want errWikiDecompressedLimit", err)
	}
}

// TestCappedReader_SurfacesThroughXMLDecoder proves the limit stops a streaming
// parse with a controlled error rather than silently truncating the document —
// the exact failure mode a decompression bomb would otherwise cause.
func TestCappedReader_SurfacesThroughXMLDecoder(t *testing.T) {
	xmlData := strings.Repeat("<page><title>x</title></page>", 1000) // far over the cap
	capped := &cappedReader{r: strings.NewReader(xmlData), limit: 50}
	dec := xml.NewDecoder(capped)

	var lastErr error
	for {
		if _, err := dec.Token(); err != nil {
			lastErr = err
			break
		}
	}
	if !errors.Is(lastErr, errWikiDecompressedLimit) {
		t.Errorf("XML decoder surfaced %v, want errWikiDecompressedLimit", lastErr)
	}
}
