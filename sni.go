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
	"path/filepath"
	"strings"
)

// Config holds the plugin configuration.
type Config struct {
	RejectOnMissingSNI  bool   `json:"rejectOnMissingSNI,omitempty"`
	RejectOnMissingHost bool   `json:"rejectOnMissingHost,omitempty"`
	LogOnly             bool   `json:"logOnly,omitempty"`
	LogLevel            string `json:"logLevel,omitempty"`
	LogFilePath         string `json:"logFilePath,omitempty"`
	LogFormat           string `json:"logFormat,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		RejectOnMissingSNI: true,
		LogLevel:           "INFO",
		LogFormat:          "common",
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

	level, err := parseLogLevel(config.LogLevel)
	if err != nil {
		return nil, err
	}

	output, err := logOutput(config.LogFilePath)
	if err != nil {
		return nil, err
	}

	handler, err := logHandler(config.LogFormat, output, level)
	if err != nil {
		return nil, err
	}

	logger := slog.New(handler).With("middleware", name)

	logger.Info("started",
		"rejectOnMissingSNI", config.RejectOnMissingSNI,
		"rejectOnMissingHost", config.RejectOnMissingHost,
		"logOnly", config.LogOnly,
	)

	return &SNIMatch{
		next:   next,
		name:   name,
		logger: logger,
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

	// If Host is missing and the config says to reject, do so.
	if host == "" && m.config.RejectOnMissingHost {
		m.reject(rw, req, "missing Host header", sni, host)
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

// reject handles a policy violation. In log-only mode, it logs the event but
// forwards the request to the next handler. Otherwise (the default), it logs
// and returns 421 Misdirected Request.
func (m *SNIMatch) reject(rw http.ResponseWriter, req *http.Request, msg, sni, host string) {
	if m.config.LogOnly {
		m.logger.Info(msg+" (log-only mode, request allowed)", "sni", sni, "host", host)
		m.next.ServeHTTP(rw, req)
		return
	}
	m.logger.Warn(msg, "sni", sni, "host", host)
	rw.WriteHeader(http.StatusMisdirectedRequest)
}

// parseLogLevel converts a string log level to a slog.Level.
func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO", "":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logLevel must be one of DEBUG, INFO, WARN, ERROR; got %q", s)
	}
}

// logOutput returns the writer for log output. If path is empty, os.Stdout is
// used. Otherwise the file at path is opened for appending.
func logOutput(path string) (*os.File, error) {
	if path == "" {
		return os.Stdout, nil
	}

	f, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("logFilePath is not writable: %w", err)
	}

	return f, nil
}

// logHandler creates a slog.Handler based on the format string. Supported
// values are "common" (slog.TextHandler) and "json" (slog.JSONHandler).
func logHandler(format string, output *os.File, level slog.Level) (slog.Handler, error) {
	opts := &slog.HandlerOptions{Level: level}

	switch format {
	case "common", "":
		return slog.NewTextHandler(output, opts), nil
	case "json":
		return slog.NewJSONHandler(output, opts), nil
	default:
		return nil, fmt.Errorf("logFormat must be \"common\" or \"json\"; got %q", format)
	}
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
