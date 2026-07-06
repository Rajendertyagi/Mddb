package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
)

func TestIsDisallowedIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},       // loopback
		{"::1", true},             // IPv6 loopback
		{"10.0.0.5", true},        // RFC1918
		{"172.16.0.1", true},      // RFC1918
		{"192.168.1.1", true},     // RFC1918
		{"169.254.169.254", true}, // link-local (cloud metadata)
		{"0.0.0.0", true},         // unspecified
		// SEC-011: ranges net.IP predicates miss.
		{"100.64.0.1", true},      // CGNAT lower bound (RFC 6598)
		{"100.100.10.10", true},   // CGNAT middle (k8s/cloud fabrics)
		{"100.127.255.254", true}, // CGNAT upper bound
		{"192.0.0.1", true},       // RFC 6890 protocol assignments
		{"198.18.0.1", true},      // RFC 2544 benchmarking
		{"198.19.255.254", true},  // RFC 2544 upper half
		{"255.255.255.255", true}, // limited broadcast
		{"8.8.8.8", false},        // public
		{"1.1.1.1", false},        // public
		{"93.184.216.34", false},  // public (example.com)
		{"100.63.255.254", false}, // just below CGNAT — stays public
		{"100.128.0.1", false},    // just above CGNAT — stays public
		{"198.17.255.254", false}, // just below benchmarking — public
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := isDisallowedIP(net.ParseIP(tt.ip)); got != tt.want {
				t.Errorf("isDisallowedIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateOutboundURL(t *testing.T) {
	t.Setenv("MDDB_OUTBOUND_ALLOW_PRIVATE", "") // assert blocking (TestMain enables it)
	blocked := []string{
		"http://127.0.0.1/x",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.1.2.3:8080/",
		"http://[::1]:9000/",
		"http://localhost/admin",
	}
	for _, raw := range blocked {
		u, _ := url.Parse(raw)
		if err := validateOutboundURL(u); !errors.Is(err, errSSRFBlocked) {
			t.Errorf("validateOutboundURL(%q) = %v, want errSSRFBlocked", raw, err)
		}
	}

	// A public literal IP passes (no network call — literal IP short-circuits).
	u, _ := url.Parse("http://8.8.8.8/")
	if err := validateOutboundURL(u); err != nil {
		t.Errorf("validateOutboundURL(public IP) = %v, want nil", err)
	}
}

func TestSafeDialContext_BlocksPrivateLiteralIP(t *testing.T) {
	t.Setenv("MDDB_OUTBOUND_ALLOW_PRIVATE", "") // assert blocking (TestMain enables it)
	// A literal private IP is rejected before any connection is attempted.
	_, err := SafeDialContext(context.Background(), "tcp", "169.254.169.254:80")
	if !errors.Is(err, errSSRFBlocked) {
		t.Errorf("SafeDialContext(metadata IP) = %v, want errSSRFBlocked", err)
	}
	_, err = SafeDialContext(context.Background(), "tcp", "127.0.0.1:80")
	if !errors.Is(err, errSSRFBlocked) {
		t.Errorf("SafeDialContext(loopback) = %v, want errSSRFBlocked", err)
	}
}

func TestSafeDialContext_AllowPrivateOptIn(t *testing.T) {
	t.Setenv("MDDB_OUTBOUND_ALLOW_PRIVATE", "true")
	// With the opt-in, the SSRF check is skipped — the dial proceeds and fails
	// with a normal connection error, NOT errSSRFBlocked.
	_, err := SafeDialContext(context.Background(), "tcp", "127.0.0.1:1")
	if errors.Is(err, errSSRFBlocked) {
		t.Error("with MDDB_OUTBOUND_ALLOW_PRIVATE=true, dial must not be SSRF-blocked")
	}
}

func TestHostExempt_Allowlist(t *testing.T) {
	t.Setenv("MDDB_OUTBOUND_ALLOW_PRIVATE", "") // isolate allowlist behaviour (TestMain enables allow-private)
	t.Setenv("MDDB_OUTBOUND_ALLOWLIST", "internal.example.com, ollama")
	if !hostExempt("internal.example.com") {
		t.Error("allowlisted host should be exempt")
	}
	if !hostExempt("ollama") {
		t.Error("allowlisted host (trimmed) should be exempt")
	}
	if hostExempt("evil.example.com") {
		t.Error("non-allowlisted host must not be exempt")
	}
}

func TestSsrfCheckRedirect(t *testing.T) {
	t.Setenv("MDDB_OUTBOUND_ALLOW_PRIVATE", "") // assert blocking (TestMain enables it)
	// Too many redirects.
	via := make([]*http.Request, 5)
	req, _ := http.NewRequest(http.MethodGet, "http://8.8.8.8/", nil)
	if err := ssrfCheckRedirect(req, via); err == nil {
		t.Error("expected error on too many redirects")
	}

	// Redirect to a private IP is blocked.
	priv, _ := http.NewRequest(http.MethodGet, "http://169.254.169.254/", nil)
	if err := ssrfCheckRedirect(priv, nil); !errors.Is(err, errSSRFBlocked) {
		t.Errorf("redirect to metadata IP = %v, want errSSRFBlocked", err)
	}

	// Redirect to a public IP is allowed.
	pub, _ := http.NewRequest(http.MethodGet, "http://8.8.8.8/", nil)
	if err := ssrfCheckRedirect(pub, nil); err != nil {
		t.Errorf("redirect to public IP = %v, want nil", err)
	}
}
