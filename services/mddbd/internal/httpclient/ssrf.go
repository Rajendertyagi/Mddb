// Package main — SSRF protection for outbound HTTP (SEC-004).
//
// Webhooks, import-url, bulk callbacks and automation triggers all dial URLs
// supplied by users. Without address checks those become Server-Side Request
// Forgery vectors: reading cloud-metadata (169.254.169.254), hitting internal
// admin panels, or port-scanning the cluster. SafeDialContext resolves the
// host and refuses private/loopback/link-local targets, then dials the
// already-resolved IP to defeat DNS rebinding. validateOutboundURL re-applies
// the same policy on each redirect hop.
//
// Internal service clients (embedding providers / Ollama) build their OWN
// http.Client and do NOT go through the shared pooled transport, so legitimate
// private-network calls keep working. Operators on trusted intranets can opt
// out with MDDB_OUTBOUND_ALLOW_PRIVATE=true or allowlist specific hosts via
// MDDB_OUTBOUND_ALLOWLIST=host1,host2.
package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var errSSRFBlocked = errors.New("destination address is not allowed (private/loopback/link-local)")

// outboundAllowPrivate reports whether private/loopback destinations are
// explicitly permitted (intranet deployments).
func outboundAllowPrivate() bool {
	return os.Getenv("MDDB_OUTBOUND_ALLOW_PRIVATE") == "true"
}

// outboundAllowlistHas reports whether host is in MDDB_OUTBOUND_ALLOWLIST.
func outboundAllowlistHas(host string) bool {
	raw := os.Getenv("MDDB_OUTBOUND_ALLOWLIST")
	if raw == "" {
		return false
	}
	host = strings.ToLower(host)
	for _, h := range strings.Split(raw, ",") {
		if strings.ToLower(strings.TrimSpace(h)) == host && host != "" {
			return true
		}
	}
	return false
}

// hostExempt reports whether SSRF checks should be skipped for host.
func hostExempt(host string) bool {
	return outboundAllowPrivate() || outboundAllowlistHas(host)
}

// isDisallowedIP reports whether ip is in a range that outbound user requests
// must not reach.
func isDisallowedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// SafeDialContext is a net.Dialer DialContext that blocks SSRF targets.
func SafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if hostExempt(host) {
		return dialer.DialContext(ctx, network, addr)
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isDisallowedIP(ip) {
			return nil, errSSRFBlocked
		}
	}
	// Dial the already-resolved IP so a rebinding attack can't swap in a
	// private address between the lookup and the connect.
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

// validateOutboundURL re-checks a (possibly redirect) URL's host against the
// SSRF policy. Literal IPs are checked directly; hostnames are resolved.
func validateOutboundURL(u *url.URL) error {
	if u == nil {
		return errSSRFBlocked
	}
	host := u.Hostname()
	if hostExempt(host) {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if isDisallowedIP(ip) {
			return errSSRFBlocked
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if isDisallowedIP(ip) {
			return errSSRFBlocked
		}
	}
	return nil
}

// ssrfCheckRedirect is the http.Client.CheckRedirect that re-validates every
// redirect hop and caps the chain length.
func ssrfCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	return validateOutboundURL(req.URL)
}
