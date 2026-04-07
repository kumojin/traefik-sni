package traefik_sni_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	traefik_sni "github.com/kumojin/traefik-sni"
)

func TestNew_NilNext(t *testing.T) {
	_, err := traefik_sni.New(context.Background(), nil, traefik_sni.CreateConfig(), "test")
	require.Error(t, err)
}

func TestNew_InvalidMode(t *testing.T) {
	next := new(MockHandler)
	config := traefik_sni.CreateConfig()
	config.Mode = "invalid"
	_, err := traefik_sni.New(context.Background(), next, config, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mode")
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
	tests := map[string]struct {
		sni            string
		host           string
		config         *traefik_sni.Config // nil = CreateConfig() defaults
		wantStatus     int
		wantNextCalled bool
	}{
		// Matching hosts — request passes through.
		"match bare":                   {"example.com", "example.com", nil, http.StatusOK, true},
		"match with standard port":     {"example.com", "example.com:443", nil, http.StatusOK, true},
		"match with non-standard port": {"example.com", "example.com:8443", nil, http.StatusOK, true},

		// Mismatching hosts — 421, next handler not called.
		"mismatch bare":      {"a.example.com", "b.example.com", nil, http.StatusMisdirectedRequest, false},
		"mismatch with port": {"a.example.com", "b.example.com:443", nil, http.StatusMisdirectedRequest, false},

		// Case insensitivity.
		"match mixed case": {"Example.COM", "example.com", nil, http.StatusOK, true},

		// Trailing FQDN dot normalization.
		"match FQDN dot in SNI":  {"example.com.", "example.com", nil, http.StatusOK, true},
		"match FQDN dot in Host": {"example.com", "example.com.", nil, http.StatusOK, true},
		"match FQDN dot both":    {"example.com.", "example.com.", nil, http.StatusOK, true},

		// Port and trailing dot together.
		"match port and FQDN dot": {"example.com", "example.com.:443", nil, http.StatusOK, true},

		// Audit mode — mismatch logged but not blocked.
		"mismatch audit mode": {"a.example.com", "b.example.com", &traefik_sni.Config{Mode: "audit"}, http.StatusOK, true},

		// Empty values — cannot compare, pass through.
		"empty SNI":  {"", "example.com", nil, http.StatusOK, true},
		"empty Host": {"example.com", "", nil, http.StatusOK, true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			next := new(MockHandler)
			next.On("ServeHTTP", mock.Anything, mock.Anything).Maybe()
			handler := newMiddleware(t, next, tc.config)

			req := httptest.NewRequest(http.MethodGet, "https://test/", nil)
			req.TLS = &tls.ConnectionState{ServerName: tc.sni}
			req.Host = tc.host
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			if tc.wantNextCalled {
				next.AssertCalled(t, "ServeHTTP", mock.Anything, mock.Anything)
			} else {
				next.AssertNotCalled(t, "ServeHTTP", mock.Anything, mock.Anything)
			}
		})
	}
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
