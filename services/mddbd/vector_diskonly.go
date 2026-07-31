package main

import (
	"sort"

	vec "mddb/internal/vector"
)

// Disk-only vector mode: for collections configured with diskOnlyVectors,
// RAM holds only the quantized representation (QuantizedVectorIndex) while
// full-precision vectors live exclusively in the BoltDB vectors bucket.
// Searches run in two phases:
//
//	phase 1: scan the quantized index in RAM for an oversampled candidate set
//	phase 2: batch-read the candidates' full vectors from disk and rescore
//	         them exactly, restoring the precision lost to quantization
//
// This trades one extra BoltDB read per query for a 4x (int8) to 8x (int4)
// smaller vector memory footprint, since the float32 flat index skips these
// collections entirely.

// collectionDiskOnly reports whether a collection keeps full vectors on disk
// only. Requires quantization to be configured — without it there is no
// compact in-memory representation to search on.
func (s *Server) collectionDiskOnly(collection string) bool {
	if s.CollectionManager == nil {
		return false
	}
	cfg, ok := s.CollectionManager.Get(collection)
	return ok && cfg.DiskOnlyVectors && cfg.Quantization != "" && cfg.Quantization != "float32"
}

// rescoreFromDisk replaces approximate quantized scores with exact scores
// computed from the full-precision vectors on disk, then re-sorts. It also
// returns the fetched vectors so downstream steps (MMR) can reuse them
// without a second disk read. Candidates whose vector is missing keep their
// approximate score.
func (s *Server) rescoreFromDisk(collection string, query []float32, results []vec.VectorResult, metric vec.SimilarityFunc) ([]vec.VectorResult, map[string][]float32) {
	if len(results) == 0 {
		return results, nil
	}
	if metric == nil {
		metric = vec.CosineSimilarity
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.DocID
	}
	vectors := s.VectorStore.GetVectors(collection, ids)
	for i := range results {
		if fv, ok := vectors[results[i].DocID]; ok {
			results[i].Score = metric(query, fv)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results, vectors
}
