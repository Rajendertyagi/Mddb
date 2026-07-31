package vector

// MMRRerank reorders search results by Maximal Marginal Relevance, balancing
// query relevance against diversity among the selected results:
//
//	mmrScore = lambda*relevance - (1-lambda)*max(similarity to already selected)
//
// lambda=1.0 reproduces the original relevance ordering; lambda=0.0 selects
// for maximum diversity. k limits how many results are selected (k<=0 selects
// all). getVector resolves a result ID to its embedding; results whose vector
// cannot be resolved incur no diversity penalty (their pairwise similarity is
// treated as 0), so mixed result sets degrade gracefully.
//
// The relevance component uses each result's existing Score, so it composes
// with whatever metric or boosting produced the input ranking.
func MMRRerank(results []VectorResult, lambda float64, k int, getVector func(id string) []float32) []VectorResult {
	if len(results) <= 1 || lambda >= 1.0 {
		if k > 0 && len(results) > k {
			return results[:k]
		}
		return results
	}
	if lambda < 0 {
		lambda = 0
	}
	if k <= 0 || k > len(results) {
		k = len(results)
	}

	// Resolve candidate vectors once.
	vectors := make([][]float32, len(results))
	for i, r := range results {
		vectors[i] = getVector(r.DocID)
	}

	selected := make([]VectorResult, 0, k)
	selectedVecs := make([][]float32, 0, k)
	remaining := make([]int, len(results))
	for i := range results {
		remaining[i] = i
	}

	for len(selected) < k && len(remaining) > 0 {
		bestPos := -1
		bestScore := float32(0)
		for pos, idx := range remaining {
			var maxSim float32
			if vectors[idx] != nil {
				for _, sv := range selectedVecs {
					if sv == nil {
						continue
					}
					if sim := CosineSimilarity(vectors[idx], sv); sim > maxSim {
						maxSim = sim
					}
				}
			}
			score := float32(lambda)*results[idx].Score - float32(1-lambda)*maxSim
			if bestPos < 0 || score > bestScore {
				bestPos = pos
				bestScore = score
			}
		}
		idx := remaining[bestPos]
		selected = append(selected, results[idx])
		selectedVecs = append(selectedVecs, vectors[idx])
		remaining = append(remaining[:bestPos], remaining[bestPos+1:]...)
	}

	return selected
}
