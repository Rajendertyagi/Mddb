package webhooks

import (
	"os"
	"testing"
)

// TestMain configures the package test environment.
//
// The webhook delivery tests post to local httptest servers (127.0.0.1), which
// the outbound SSRF guard (SEC-004) blocks by default. Enable the documented
// MDDB_OUTBOUND_ALLOW_PRIVATE opt-in for the whole test binary so those local
// deliveries succeed. The guard's blocking behaviour is verified independently
// in internal/httpclient.
func TestMain(m *testing.M) {
	if os.Getenv("MDDB_OUTBOUND_ALLOW_PRIVATE") == "" {
		_ = os.Setenv("MDDB_OUTBOUND_ALLOW_PRIVATE", "true")
	}
	os.Exit(m.Run())
}
