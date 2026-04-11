// Package main — TLS / mTLS configuration helpers.
//
// buildServerTLSConfig turns the user-facing TLSConfig (server_config.go)
// into a crypto/tls.Config ready to attach to net/http.Server.
// Supports optional mutual TLS via a client-CA PEM bundle.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

// buildServerTLSConfig loads the server cert/key and, if configured, a
// client-CA bundle for mTLS. Returns nil,nil if TLS is disabled or missing
// required fields (caller must check cfg.Enabled first).
func buildServerTLSConfig(cfg TLSConfig) (*tls.Config, error) {
	if !cfg.Enabled || cfg.CertFile == "" || cfg.KeyFile == "" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server keypair: %w", err)
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	if cfg.ClientCAFile == "" {
		return tlsCfg, nil
	}

	caPEM, err := os.ReadFile(cfg.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("read client CA bundle %q: %w", cfg.ClientCAFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("client CA bundle %q contains no valid PEM certificates", cfg.ClientCAFile)
	}
	tlsCfg.ClientCAs = pool

	switch strings.ToLower(cfg.ClientAuth) {
	case "request":
		tlsCfg.ClientAuth = tls.VerifyClientCertIfGiven
	case "", "require":
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	default:
		return nil, fmt.Errorf("unknown tls.clientAuth %q (use 'require' or 'request')", cfg.ClientAuth)
	}
	return tlsCfg, nil
}
