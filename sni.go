// Package traefik_sni is a Traefik middleware plugin that compares the TLS SNI
// server name with the HTTP Host header and returns 421 Misdirected Request
// when they do not match. This prevents domain fronting attacks.
package traefik_sni

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

// Config holds the plugin configuration.
type Config struct {
	// AllowedHosts is an optional list of hostnames that are exempt from the
	// SNI/Host match check. Useful for health-check endpoints or special cases.
	AllowedHosts []string `json:"allowedHosts,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

// SNIMatch is the middleware that enforces SNI/Host header consistency.
type SNIMatch struct {
	next         http.Handler
	name         string
	allowedHosts map[string]struct{}
	logger       *log.Logger
}

// New creates a new SNIMatch middleware instance.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if next == nil {
		return nil, fmt.Errorf("next handler cannot be nil")
	}

	allowed := make(map[string]struct{}, len(config.AllowedHosts))
	for _, h := range config.AllowedHosts {
		allowed[normalizeHost(h)] = struct{}{}
	}

	return &SNIMatch{
		next:         next,
		name:         name,
		allowedHosts: allowed,
		logger:       log.New(os.Stdout, fmt.Sprintf("[%s] ", name), log.Ldate|log.Ltime),
	}, nil
}

// ServeHTTP implements http.Handler. It compares the TLS SNI value with the
// HTTP Host header and returns 421 Misdirected Request on mismatch.
func (m *SNIMatch) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// No TLS info means the request was not received over TLS (or TLS was
	// terminated elsewhere). Pass through.
	if req.TLS == nil {
		m.next.ServeHTTP(rw, req)
		return
	}

	sni := normalizeHost(req.TLS.ServerName)
	host := normalizeHost(req.Host)

	// If either value is empty, we cannot make a comparison. Pass through.
	// This handles cases like health checks using IPs (no SNI) or edge cases
	// where the host is somehow empty.
	if sni == "" || host == "" {
		m.next.ServeHTTP(rw, req)
		return
	}

	// Check the allowlist before rejecting.
	if _, ok := m.allowedHosts[host]; ok {
		m.next.ServeHTTP(rw, req)
		return
	}

	if sni != host {
		m.logger.Printf("misdirected request: SNI=%q Host=%q", sni, host)
		rw.WriteHeader(http.StatusMisdirectedRequest)
		return
	}

	m.next.ServeHTTP(rw, req)
}

// normalizeHost extracts the hostname, strips the port if present, removes a
// trailing dot, and lowercases the result. This ensures consistent comparison
// between SNI and Host values.
func normalizeHost(raw string) string {
	// Host header may be "host:port"; SNI should not contain a port but we
	// normalize both paths for safety.
	h := raw
	if strings.Contains(h, ":") {
		var err error
		h, _, err = net.SplitHostPort(h)
		if err != nil {
			// SplitHostPort fails when there is no port (e.g. bare IPv6 or
			// just a hostname without a colon). In that case keep original.
			h = raw
		}
	}

	// Remove trailing dot (FQDN form).
	h = strings.TrimSuffix(h, ".")

	return strings.ToLower(h)
}
