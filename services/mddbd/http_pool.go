package main

import (
	"net/http"
	"time"
)

// SharedHTTPClient is a pre-configured http.Client with connection pooling.
// It reuses TCP connections via a shared transport, reducing latency and resource usage
// for outbound requests (webhooks, import-url, automation triggers).
//
// Configuration via environment variables:
//
//	MDDB_HTTP_POOL_MAX_IDLE      - Max idle connections total (default: 100)
//	MDDB_HTTP_POOL_MAX_PER_HOST  - Max idle connections per host (default: 10)
//	MDDB_HTTP_POOL_IDLE_TIMEOUT  - Idle connection timeout in seconds (default: 90)
var SharedHTTPClient *http.Client

func init() {
	maxIdle := envDefaultInt("MDDB_HTTP_POOL_MAX_IDLE", 100)
	maxPerHost := envDefaultInt("MDDB_HTTP_POOL_MAX_PER_HOST", 10)
	idleTimeout := envDefaultInt("MDDB_HTTP_POOL_IDLE_TIMEOUT", 90)

	transport := &http.Transport{
		// SEC-004: safeDialContext blocks SSRF targets (private/loopback/
		// link-local / cloud-metadata) and dials a pre-resolved IP to defeat
		// DNS rebinding. This shared transport backs all user-URL outbound
		// paths (webhooks, import-url, automation, bulk callbacks); internal
		// embedding providers use their own clients and are unaffected.
		DialContext:         safeDialContext,
		MaxIdleConns:        maxIdle,
		MaxIdleConnsPerHost: maxPerHost,
		IdleConnTimeout:     time.Duration(idleTimeout) * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
	}

	SharedHTTPClient = &http.Client{
		Transport:     transport,
		Timeout:       30 * time.Second,
		CheckRedirect: ssrfCheckRedirect,
	}
}

// NewPooledClientWithTimeout returns the shared pooled client's transport
// wrapped in a new http.Client with a custom timeout.
// This reuses the same connection pool (and SSRF-safe dialer) but allows
// per-use timeout control.
func NewPooledClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport:     SharedHTTPClient.Transport,
		Timeout:       timeout,
		CheckRedirect: ssrfCheckRedirect,
	}
}
