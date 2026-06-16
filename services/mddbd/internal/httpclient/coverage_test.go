package httpclient

import (
	"context"
	"errors"
	"net/url"
	"testing"
)

func TestSafeDialContextBranches(t *testing.T) {
	ctx := context.Background()

	// Malformed address -> SplitHostPort error.
	if _, err := SafeDialContext(ctx, "tcp", "noport"); err == nil {
		t.Error("expected error for address without port")
	}
	// Loopback is resolved and rejected before any dial.
	if _, err := SafeDialContext(ctx, "tcp", "127.0.0.1:80"); !errors.Is(err, errSSRFBlocked) {
		t.Errorf("loopback dial err = %v, want errSSRFBlocked", err)
	}

	// With private addresses allowed, an exempt host is dialed (and fails to
	// connect) rather than SSRF-blocked.
	t.Setenv("MDDB_OUTBOUND_ALLOW_PRIVATE", "true")
	if _, err := SafeDialContext(ctx, "tcp", "127.0.0.1:1"); errors.Is(err, errSSRFBlocked) {
		t.Error("exempt host should not be SSRF-blocked")
	}
}

func TestValidateOutboundURLBranches(t *testing.T) {
	mustURL := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}

	if err := validateOutboundURL(nil); !errors.Is(err, errSSRFBlocked) {
		t.Errorf("nil URL err = %v, want errSSRFBlocked", err)
	}
	if err := validateOutboundURL(mustURL("http://127.0.0.1/x")); !errors.Is(err, errSSRFBlocked) {
		t.Errorf("loopback literal err = %v, want errSSRFBlocked", err)
	}
	if err := validateOutboundURL(mustURL("http://8.8.8.8/x")); err != nil {
		t.Errorf("public literal IP should pass, got %v", err)
	}

	t.Setenv("MDDB_OUTBOUND_ALLOW_PRIVATE", "true")
	if err := validateOutboundURL(mustURL("http://10.0.0.1/x")); err != nil {
		t.Errorf("exempt private host should pass, got %v", err)
	}
}
