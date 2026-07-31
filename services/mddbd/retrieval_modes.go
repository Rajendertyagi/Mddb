package main

import (
	"strconv"
	"strings"

	"mddb/internal/envconf"
)

// Retrieval modes for vector and hybrid search.
//
// Chunks are embedded and indexed individually (key "docID#N"), but callers
// have different context needs:
//
//   - "parent": one result per parent document, scored by its best-matching
//     chunk. This is the historical behavior and stays the default.
//   - "chunk": one result per matching chunk, carrying the exact passage that
//     matched (chunkIndex + chunkText) — precise context for LLM prompts.
//   - "window": like "chunk", but the passage is widened with N neighboring
//     chunks on each side, trading precision for surrounding context.
//
// Chunking is deterministic (ChunkText on the parent's content with the
// configured chunk size), so passages are re-derived from the parent document
// at query time — no extra storage, and always in sync with the content.
const (
	RetrievalModeParent = "parent"
	RetrievalModeChunk  = "chunk"
	RetrievalModeWindow = "window"
)

// validRetrievalMode reports whether mode is a supported retrieval mode.
// The empty string is valid and means RetrievalModeParent.
func validRetrievalMode(mode string) bool {
	switch mode {
	case "", RetrievalModeParent, RetrievalModeChunk, RetrievalModeWindow:
		return true
	}
	return false
}

// splitChunkKey splits an index key "docID#N" into the parent document ID and
// the chunk index. Legacy non-chunked keys map to chunk 0.
func splitChunkKey(key string) (docID string, chunkIndex int) {
	hashIdx := strings.LastIndexByte(key, '#')
	if hashIdx < 0 {
		return key, 0
	}
	n, err := strconv.Atoi(key[hashIdx+1:])
	if err != nil || n < 0 {
		return key, 0
	}
	return key[:hashIdx], n
}

// chunkPassage re-derives the chunk (or window of chunks) text from the
// parent document's content. windowSize is the number of neighboring chunks
// included on each side; 0 returns just the matching chunk.
func chunkPassage(contentMD string, chunkIndex, windowSize int) string {
	chunkSize := envconf.Int("MDDB_EMBEDDING_CHUNK_SIZE", 1500)
	chunks := ChunkText(contentMD, chunkSize)
	if len(chunks) == 0 {
		return ""
	}
	if chunkIndex >= len(chunks) {
		chunkIndex = len(chunks) - 1
	}
	if windowSize <= 0 {
		return chunks[chunkIndex]
	}
	lo := chunkIndex - windowSize
	if lo < 0 {
		lo = 0
	}
	hi := chunkIndex + windowSize + 1
	if hi > len(chunks) {
		hi = len(chunks)
	}
	return strings.Join(chunks[lo:hi], "\n\n")
}
