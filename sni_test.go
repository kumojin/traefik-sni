package traefik_sni_host_check_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	traefik_sni "github.com/DialogInsight/traefik-sni-host-check"
)

func TestNew_NilNext(t *testing.T) {
	_, err := traefik_sni.New(context.Background(), nil, traefik_sni.CreateConfig(), "test")
	require.Error(t, err)
}

func TestNew_NilConfig(t *testing.T) {
	next := new(MockHandler)
	handler, err := traefik_sni.New(context.Background(), next, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, handler)
}

func TestNew_InvalidLogLevel(t *testing.T) {
	next := new(MockHandler)
	config := traefik_sni.CreateConfig()
	config.LogLevel = "TRACE"

	_, err := traefik_sni.New(context.Background(), next, config, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logLevel")
}

func TestNew_InvalidLogFormat(t *testing.T) {
	next := new(MockHandler)
	config := traefik_sni.CreateConfig()
	config.LogFormat = "yaml"

	_, err := traefik_sni.New(context.Background(), next, config, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logFormat")
}

func TestNew_InvalidLogFilePath(t *testing.T) {
	next := new(MockHandler)
	config := traefik_sni.CreateConfig()
	config.LogFilePath = "/nonexistent/directory/test.log"

	_, err := traefik_sni.New(context.Background(), next, config, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logFilePath")
}

func TestNew_ValidLogFilePath(t *testing.T) {
	next := new(MockHandler)
	config := traefik_sni.CreateConfig()
	config.LogFilePath = filepath.Join(t.TempDir(), "test.log")

	handler, err := traefik_sni.New(context.Background(), next, config, "test")
	require.NoError(t, err)
	assert.NotNil(t, handler)
}

func TestNew_LogFormatJSON(t *testing.T) {
	next := new(MockHandler)
	config := traefik_sni.CreateConfig()
	config.LogFormat = "json"

	handler, err := traefik_sni.New(context.Background(), next, config, "test")
	require.NoError(t, err)
	assert.NotNil(t, handler)
}

func TestNew_LogLevelCaseInsensitive(t *testing.T) {
	next := new(MockHandler)
	config := traefik_sni.CreateConfig()
	config.LogLevel = "debug"

	handler, err := traefik_sni.New(context.Background(), next, config, "test")
	require.NoError(t, err)
	assert.NotNil(t, handler)
}

func TestServeHTTP_NoTLS(t *testing.T) {
	next := new(MockHandler)
	next.On("ServeHTTP", mock.Anything, mock.Anything).Once()
	handler := newMiddleware(t, next, nil)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	next.AssertExpectations(t)
}

func TestServeHTTP(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sni            string
		host           string
		config         *traefik_sni.Config // nil = CreateConfig() defaults
		wantStatus     int
		wantBody       string // expected substring in response body (empty = don't check)
		wantNextCalled bool
	}{
		// Matching hosts — request passes through.
		"match bare":                   {"example.com", "example.com", nil, http.StatusOK, "", true},
		"match with standard port":     {"example.com", "example.com:443", nil, http.StatusOK, "", true},
		"match with non-standard port": {"example.com", "example.com:8443", nil, http.StatusOK, "", true},

		// Mismatching hosts — 421, next handler not called.
		"mismatch bare":      {"a.example.com", "b.example.com", nil, http.StatusMisdirectedRequest, "421 misdirected request", false},
		"mismatch with port": {"a.example.com", "b.example.com:443", nil, http.StatusMisdirectedRequest, "421 misdirected request", false},

		// Case insensitivity.
		"match mixed case": {"Example.COM", "example.com", nil, http.StatusOK, "", true},

		// Trailing FQDN dot normalization.
		"match FQDN dot in SNI":  {"example.com.", "example.com", nil, http.StatusOK, "", true},
		"match FQDN dot in Host": {"example.com", "example.com.", nil, http.StatusOK, "", true},
		"match FQDN dot both":    {"example.com.", "example.com.", nil, http.StatusOK, "", true},

		// Port and trailing dot together.
		"match port and FQDN dot": {"example.com", "example.com.:443", nil, http.StatusOK, "", true},

		// Log-only mode — mismatch logged but not blocked.
		"mismatch log-only mode": {"a.example.com", "b.example.com", &traefik_sni.Config{LogOnly: true}, http.StatusOK, "", true},

		// Empty values — cannot compare, pass through.
		"empty SNI rejected by default": {"", "example.com", nil, http.StatusMisdirectedRequest, "421 misdirected request", false},
		"empty Host allowed by default": {"example.com", "", nil, http.StatusOK, "", true},

		// Empty SNI -- allowed when rejectOnMissingSNI=false.
		"empty SNI allowed when configured": {
			"", "example.com",
			&traefik_sni.Config{RejectOnMissingSNI: false},
			http.StatusOK, "", true,
		},

		// Empty SNI -- log-only mode logs but allows.
		"empty SNI log-only mode": {
			"", "example.com",
			&traefik_sni.Config{LogOnly: true, RejectOnMissingSNI: true},
			http.StatusOK, "", true,
		},

		// Empty Host -- rejected when configured.
		"empty Host rejected when configured": {
			"example.com", "",
			&traefik_sni.Config{RejectOnMissingSNI: true, RejectOnMissingHost: true},
			http.StatusMisdirectedRequest, "421 misdirected request", false,
		},

		// Empty Host -- log-only mode logs but allows.
		"empty Host log-only mode": {
			"example.com", "",
			&traefik_sni.Config{LogOnly: true, RejectOnMissingSNI: true, RejectOnMissingHost: true},
			http.StatusOK, "", true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			next := new(MockHandler)
			next.On("ServeHTTP", mock.Anything, mock.Anything).Maybe()
			handler := newMiddleware(t, next, tc.config)

			req := httptest.NewRequest(http.MethodGet, "https://test/", nil)
			req.TLS = &tls.ConnectionState{ServerName: tc.sni}
			req.Host = tc.host
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			if tc.wantBody != "" {
				assert.Contains(t, rr.Body.String(), tc.wantBody)
			}
			if tc.wantNextCalled {
				next.AssertCalled(t, "ServeHTTP", mock.Anything, mock.Anything)
			} else {
				next.AssertNotCalled(t, "ServeHTTP", mock.Anything, mock.Anything)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Log output tests — verify log content via logFilePath.
// ---------------------------------------------------------------------------

func TestLogOutput_Startup(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	config := traefik_sni.CreateConfig()
	config.LogFilePath = logFile
	config.LogLevel = "DEBUG"

	next := new(MockHandler)
	_, err := traefik_sni.New(context.Background(), next, config, "test-sni")
	require.NoError(t, err)

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)

	log := string(content)
	// INFO line: plugin started.
	assert.Contains(t, log, `msg="plugin started"`)
	assert.Contains(t, log, "middleware=test-sni")
	// DEBUG line: full configuration.
	assert.Contains(t, log, "msg=configuration")
	assert.Contains(t, log, "rejectOnMissingSNI=true")
	assert.Contains(t, log, "rejectOnMissingHost=false")
	assert.Contains(t, log, "logOnly=false")
	assert.Contains(t, log, "logLevel=DEBUG")
	assert.Contains(t, log, "logFormat=common")
}

func TestLogOutput_MismatchCommonFormat(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	config := traefik_sni.CreateConfig()
	config.LogFilePath = logFile

	next := new(MockHandler)
	handler, err := traefik_sni.New(context.Background(), next, config, "test-sni")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://test/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "a.example.com"}
	req.Host = "b.example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)

	log := string(content)
	assert.Contains(t, log, "level=WARN")
	assert.Contains(t, log, `msg="misdirected request"`)
	assert.Contains(t, log, "sni=a.example.com")
	assert.Contains(t, log, "host=b.example.com")
}

func TestLogOutput_MismatchJSONFormat(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	config := traefik_sni.CreateConfig()
	config.LogFilePath = logFile
	config.LogFormat = "json"

	next := new(MockHandler)
	handler, err := traefik_sni.New(context.Background(), next, config, "test-sni")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://test/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "a.example.com"}
	req.Host = "b.example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)

	var found bool
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["msg"] == "misdirected request" {
			assert.Equal(t, "WARN", entry["level"])
			assert.Equal(t, "a.example.com", entry["sni"])
			assert.Equal(t, "b.example.com", entry["host"])
			found = true
			break
		}
	}
	assert.True(t, found, "expected JSON log entry for 'misdirected request'")
}

func TestLogOutput_LevelSuppressesBelow(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	config := traefik_sni.CreateConfig()
	config.LogFilePath = logFile
	config.LogLevel = "ERROR"

	next := new(MockHandler)
	handler, err := traefik_sni.New(context.Background(), next, config, "test-sni")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://test/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "a.example.com"}
	req.Host = "b.example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)

	// Both startup (INFO), config (DEBUG), and violation (WARN) are below ERROR — file should be empty.
	assert.Empty(t, string(content))
}

func TestLogOutput_LevelAllowsAtThreshold(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	config := traefik_sni.CreateConfig()
	config.LogFilePath = logFile
	config.LogLevel = "WARN"

	next := new(MockHandler)
	handler, err := traefik_sni.New(context.Background(), next, config, "test-sni")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://test/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "a.example.com"}
	req.Host = "b.example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)

	log := string(content)
	// Violation (WARN) should appear.
	assert.Contains(t, log, "level=WARN")
	assert.Contains(t, log, `msg="misdirected request"`)
	// Startup (INFO) should NOT appear — below WARN threshold.
	assert.NotContains(t, log, `msg="plugin started"`)
	// Config (DEBUG) should NOT appear either.
	assert.NotContains(t, log, "msg=configuration")
}

func TestLogOutput_LogOnlyMode(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	config := traefik_sni.CreateConfig()
	config.LogFilePath = logFile
	config.LogOnly = true

	next := new(MockHandler)
	next.On("ServeHTTP", mock.Anything, mock.Anything).Once()
	handler, err := traefik_sni.New(context.Background(), next, config, "test-sni")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://test/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "a.example.com"}
	req.Host = "b.example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)

	log := string(content)
	assert.Contains(t, log, "log-only mode, request allowed")
	assert.Contains(t, log, "level=WARN")
	next.AssertExpectations(t)
}

func newMiddleware(t *testing.T, next http.Handler, config *traefik_sni.Config) http.Handler {
	t.Helper()
	if config == nil {
		config = traefik_sni.CreateConfig()
	}
	handler, err := traefik_sni.New(context.Background(), next, config, "test-sni-match")
	require.NoError(t, err)
	return handler
}

// MockHandler stands in for the next handler in the middleware chain.
// It implements http.Handler via testify/mock, which lets tests verify
// whether the middleware forwarded the request (AssertCalled) or blocked
// it (AssertNotCalled) without needing a real downstream handler.
type MockHandler struct {
	mock.Mock
}

func (m *MockHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	m.Called(rw, req)
	rw.WriteHeader(http.StatusOK)
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// noopHandler is a minimal http.Handler for benchmarks, avoiding testify/mock
// overhead in hot loops.
type noopHandler struct{}

func (noopHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

func BenchmarkServeHTTP_Match(b *testing.B) {
	config := traefik_sni.CreateConfig()
	handler, err := traefik_sni.New(context.Background(), noopHandler{}, config, "bench")
	if err != nil {
		b.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "example.com"}
	req.Host = "example.com"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkServeHTTP_Mismatch(b *testing.B) {
	config := traefik_sni.CreateConfig()
	handler, err := traefik_sni.New(context.Background(), noopHandler{}, config, "bench")
	if err != nil {
		b.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "a.example.com"}
	req.Host = "b.example.com"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkServeHTTP_NoTLS(b *testing.B) {
	config := traefik_sni.CreateConfig()
	handler, err := traefik_sni.New(context.Background(), noopHandler{}, config, "bench")
	if err != nil {
		b.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}
