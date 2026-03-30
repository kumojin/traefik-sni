package traefik_sni_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	traefik_sni "github.com/kumojin/traefik-sni"
)

// noopHandler is a handler that records whether it was called.
type noopHandler struct {
	called bool
}

func (h *noopHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h.called = true
	rw.WriteHeader(http.StatusOK)
}

func newMiddleware(t *testing.T, config *traefik_sni.Config) (http.Handler, *noopHandler) {
	t.Helper()
	next := &noopHandler{}
	handler, err := traefik_sni.New(context.Background(), next, config, "test-sni-match")
	if err != nil {
		t.Fatalf("unexpected error creating middleware: %v", err)
	}
	return handler, next
}

func TestServeHTTP_NoTLS(t *testing.T) {
	handler, next := newMiddleware(t, traefik_sni.CreateConfig())
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !next.called {
		t.Error("expected next handler to be called")
	}
}

func TestServeHTTP_MatchingHosts(t *testing.T) {
	handler, next := newMiddleware(t, traefik_sni.CreateConfig())
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "example.com"}
	req.Host = "example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !next.called {
		t.Error("expected next handler to be called")
	}
}

func TestServeHTTP_Mismatch(t *testing.T) {
	handler, next := newMiddleware(t, traefik_sni.CreateConfig())
	req := httptest.NewRequest(http.MethodGet, "https://front.example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "front.example.com"}
	req.Host = "victim.example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMisdirectedRequest {
		t.Errorf("expected 421, got %d", rr.Code)
	}
	if next.called {
		t.Error("next handler should not be called on mismatch")
	}
}

func TestServeHTTP_HostWithPort(t *testing.T) {
	handler, next := newMiddleware(t, traefik_sni.CreateConfig())
	req := httptest.NewRequest(http.MethodGet, "https://example.com:443/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "example.com"}
	req.Host = "example.com:443"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (port should be stripped)", rr.Code)
	}
	if !next.called {
		t.Error("expected next handler to be called")
	}
}

func TestServeHTTP_MismatchWithPort(t *testing.T) {
	handler, next := newMiddleware(t, traefik_sni.CreateConfig())
	req := httptest.NewRequest(http.MethodGet, "https://front.example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "front.example.com"}
	req.Host = "victim.example.com:443"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMisdirectedRequest {
		t.Errorf("expected 421, got %d", rr.Code)
	}
	if next.called {
		t.Error("next handler should not be called on mismatch")
	}
}

func TestServeHTTP_CaseInsensitive(t *testing.T) {
	handler, next := newMiddleware(t, traefik_sni.CreateConfig())
	req := httptest.NewRequest(http.MethodGet, "https://Example.COM/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "EXAMPLE.com"}
	req.Host = "example.COM"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (comparison should be case-insensitive)", rr.Code)
	}
	if !next.called {
		t.Error("expected next handler to be called")
	}
}

func TestServeHTTP_TrailingDot(t *testing.T) {
	handler, next := newMiddleware(t, traefik_sni.CreateConfig())
	req := httptest.NewRequest(http.MethodGet, "https://example.com./", nil)
	req.TLS = &tls.ConnectionState{ServerName: "example.com."}
	req.Host = "example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (trailing dot should be stripped)", rr.Code)
	}
	if !next.called {
		t.Error("expected next handler to be called")
	}
}

func TestServeHTTP_EmptySNI(t *testing.T) {
	handler, next := newMiddleware(t, traefik_sni.CreateConfig())
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: ""}
	req.Host = "example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (empty SNI should pass through)", rr.Code)
	}
	if !next.called {
		t.Error("expected next handler to be called when SNI is empty")
	}
}

func TestServeHTTP_EmptyHost(t *testing.T) {
	handler, next := newMiddleware(t, traefik_sni.CreateConfig())
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "example.com"}
	req.Host = ""
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (empty Host should pass through)", rr.Code)
	}
	if !next.called {
		t.Error("expected next handler to be called when Host is empty")
	}
}

func TestServeHTTP_AllowedHost(t *testing.T) {
	config := traefik_sni.CreateConfig()
	config.AllowedHosts = []string{"special.example.com"}
	handler, next := newMiddleware(t, config)

	req := httptest.NewRequest(http.MethodGet, "https://front.example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "front.example.com"}
	req.Host = "special.example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (host is in allowlist)", rr.Code)
	}
	if !next.called {
		t.Error("expected next handler to be called for allowed host")
	}
}

func TestServeHTTP_AllowedHostCaseInsensitive(t *testing.T) {
	config := traefik_sni.CreateConfig()
	config.AllowedHosts = []string{"Special.Example.COM"}
	handler, next := newMiddleware(t, config)

	req := httptest.NewRequest(http.MethodGet, "https://front.example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "front.example.com"}
	req.Host = "special.example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (allowlist should be case-insensitive)", rr.Code)
	}
	if !next.called {
		t.Error("expected next handler to be called for allowed host")
	}
}

func TestServeHTTP_NotInAllowedHost(t *testing.T) {
	config := traefik_sni.CreateConfig()
	config.AllowedHosts = []string{"special.example.com"}
	handler, next := newMiddleware(t, config)

	req := httptest.NewRequest(http.MethodGet, "https://front.example.com/", nil)
	req.TLS = &tls.ConnectionState{ServerName: "front.example.com"}
	req.Host = "victim.example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMisdirectedRequest {
		t.Errorf("expected 421, got %d", rr.Code)
	}
	if next.called {
		t.Error("next handler should not be called on mismatch")
	}
}

func TestNew_NilNext(t *testing.T) {
	_, err := traefik_sni.New(context.Background(), nil, traefik_sni.CreateConfig(), "test")
	if err == nil {
		t.Error("expected error when next handler is nil")
	}
}

// TestNormalizeHost tests the normalizeHost function indirectly through
// ServeHTTP by exercising various host formats.
func TestServeHTTP_HostPortVariations(t *testing.T) {
	tests := []struct {
		name       string
		sni        string
		host       string
		wantStatus int
	}{
		{"match bare", "example.com", "example.com", http.StatusOK},
		{"match with standard port", "example.com", "example.com:443", http.StatusOK},
		{"match with non-standard port", "example.com", "example.com:8443", http.StatusOK},
		{"mismatch bare", "a.example.com", "b.example.com", http.StatusMisdirectedRequest},
		{"mismatch with port", "a.example.com", "b.example.com:443", http.StatusMisdirectedRequest},
		{"match FQDN dot in SNI", "example.com.", "example.com", http.StatusOK},
		{"match FQDN dot in Host", "example.com", "example.com.", http.StatusOK},
		{"match FQDN dot both", "example.com.", "example.com.", http.StatusOK},
		{"match mixed case", "Example.COM", "example.com", http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := newMiddleware(t, traefik_sni.CreateConfig())
			req := httptest.NewRequest(http.MethodGet, "https://test/", nil)
			req.TLS = &tls.ConnectionState{ServerName: tc.sni}
			req.Host = tc.host
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("SNI=%q Host=%q: expected %d, got %d", tc.sni, tc.host, tc.wantStatus, rr.Code)
			}
		})
	}
}
