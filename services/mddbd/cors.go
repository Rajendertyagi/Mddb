package main

import (
	"net/http"
	"os"
	"strings"
)

// corsConfig resolves the Cross-Origin policy (SEC-008). Either a wildcard `*`
// (read-only, no credentials) or an exact-match allowlist of origins. Reflecting
// an arbitrary request Origin — as the old MCP transport did — is never allowed,
// since with the Authorization / X-API-Key headers enabled that lets any site
// read responses from a user's local/intranet MDDB instance.
type corsConfig struct {
	wildcard bool
	allowed  map[string]bool
}

// parseCORSOrigins builds a config from a raw value. "*" (or empty) is wildcard;
// otherwise a comma-separated allowlist of exact origins.
func parseCORSOrigins(raw string) corsConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "*" {
		return corsConfig{wildcard: true}
	}
	cfg := corsConfig{allowed: make(map[string]bool)}
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.allowed[o] = true
		}
	}
	return cfg
}

// envCORSConfig reads MDDB_CORS_ORIGINS (CSV allowlist), falling back to the
// legacy single-value MDDB_CORS_ORIGIN, then to wildcard.
func envCORSConfig() corsConfig {
	raw := os.Getenv("MDDB_CORS_ORIGINS")
	if raw == "" {
		raw = os.Getenv("MDDB_CORS_ORIGIN")
	}
	return parseCORSOrigins(raw)
}

// applyOrigin sets Access-Control-Allow-Origin for a request. A wildcard config
// emits `*`; an allowlist emits the request Origin only on an exact match (plus
// `Vary: Origin` so caches don't serve one origin's response to another). A
// disallowed origin gets no header at all — the browser then blocks the read.
func (c corsConfig) applyOrigin(w http.ResponseWriter, requestOrigin string) {
	switch {
	case c.wildcard:
		w.Header().Set("Access-Control-Allow-Origin", "*")
	case requestOrigin != "" && c.allowed[requestOrigin]:
		w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
		w.Header().Add("Vary", "Origin")
	}
}
