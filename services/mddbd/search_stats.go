package main

// SearchStats provides performance telemetry for search responses.
type SearchStats struct {
	DurationMs  float64  `json:"durationMs"`
	QueryTerms  []string `json:"queryTerms,omitempty"`
	IndexSize   int      `json:"indexSize"`
	TotalTokens int      `json:"totalTokens"`
}

func searchStatsEnabled() bool {
	return env("MDDB_SEARCH_STATS", "true") != "false"
}
