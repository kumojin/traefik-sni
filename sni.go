// Package traefik_sni is a Traefik middleware plugin that compares the TLS SNI
// server name with the HTTP Host header and returns 421 Misdirected Request
// when they do not match. This prevents domain fronting attacks.
package traefik_sni

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
)

// Config holds the plugin configuration.
type Config struct {
	Mode               string `json:"mode,omitempty"`
	RejectOnMissingSNI bool   `json:"rejectOnMissingSNI,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		Mode:               "enforce",
		RejectOnMissingSNI: true,
	}
}

// SNIMatch is the middleware that enforces SNI/Host header consistency.
type SNIMatch struct {
	next   http.Handler
	name   string
	logger *slog.Logger
	config *Config
}

// New creates a new SNIMatch middleware instance.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if next == nil {
		return nil, fmt.Errorf("next handler cannot be nil")
	}

	if config.Mode != "enforce" && config.Mode != "audit" {
		return nil, fmt.Errorf("invalid mode %q: must be \"enforce\" or \"audit\"", config.Mode)
	}

	return &SNIMatch{
		next:   next,
		name:   name,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)).With("middleware", name),
		config: config,
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

	// If SNI is missing and the config says to reject, do so.
	if sni == "" && m.config.RejectOnMissingSNI {
		m.reject(rw, req, "missing SNI", sni, host)
		return
	}

	// If either value is empty, we cannot make a comparison. Pass through.
	// This handles cases like health checks using IPs (no SNI) or edge cases
	// where the host is somehow empty.
	if sni == "" || host == "" {
		m.next.ServeHTTP(rw, req)
		return
	}

	if sni != host {
		m.reject(rw, req, "misdirected request", sni, host)
		return
	}

	m.next.ServeHTTP(rw, req)
}

// reject handles a policy violation. In audit mode, it logs the event but
// forwards the request to the next handler. In enforce mode (the default),
// it logs and returns 421 Misdirected Request.
func (m *SNIMatch) reject(rw http.ResponseWriter, req *http.Request, msg, sni, host string) {
	if m.config.Mode == "audit" {
		m.logger.Warn(msg+" (audit mode, request allowed)", "sni", sni, "host", host)
		m.next.ServeHTTP(rw, req)
		return
	}
	m.logger.Warn(msg, "sni", sni, "host", host)
	rw.WriteHeader(http.StatusMisdirectedRequest)
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
