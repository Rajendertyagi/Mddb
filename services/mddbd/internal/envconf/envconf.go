// Package envconf provides small helpers for reading configuration from
// environment variables with typed fallbacks. It has no dependencies on the
// server, so any package may import it to resolve its own settings.
package envconf

import (
	"fmt"
	"os"
)

// String returns the value of env var key, or def when the variable is unset
// or empty.
func String(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Int returns the value of env var key parsed as an int, falling back to def
// when the variable is unset, empty, or unparseable.
func Int(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

// Int64 returns the value of env var key parsed as an int64, falling back to
// def when the variable is unset, empty, or unparseable. Used for byte-size
// limits that may exceed 32-bit range.
func Int64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int64
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}
