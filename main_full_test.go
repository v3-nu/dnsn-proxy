package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const e2eSuffix = "proxy.test"

// backendPort extracts the numeric port from an httptest.Server URL.
func backendPort(s *httptest.Server) int {
	raw := s.URL // "http://127.0.0.1:PORT" or "https://..."
	idx := strings.LastIndex(raw, ":")
	port, _ := strconv.Atoi(raw[idx+1:])
	return port
}

// domain builds a test FQDN that the proxy will accept.
//
// Format: [proto]<port>.10-0-0-1.<suffix>
//
// The IP portion (10-0-0-1) is not used for routing; any four valid octets work.
// proto is one of "ssl", "tls", "https", or "" for plain HTTP.
func domain(port int, proto string) string {
	return proto + strconv.Itoa(port) + ".10-0-0-1." + e2eSuffix
}

// nopNext is a no-op caddyhttp.Handler used as the "next" argument to ServeHTTP.
// Our handler never calls next, but the interface must be satisfied.
var nopNext = caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error {
	return nil
})

// newProxy returns an httptest.Server that wraps DNSNProxyHandler.
// It bypasses Caddy's lifecycle by setting the unexported fields directly (same package).
func newProxy(t *testing.T, backend string, insecure bool) *httptest.Server {
	t.Helper()
	h := &DNSNProxyHandler{
		Suffix:          e2eSuffix,
		Backend:         backend,
		InsecureBackend: insecure,
		re:              buildRegex(e2eSuffix),
		logger:          zap.NewNop(),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = h.ServeHTTP(w, r, nopNext)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// get sends a GET request to the proxy, overriding the Host header.
func get(t *testing.T, proxy *httptest.Server, host, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, proxy.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = host
	resp, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return strings.TrimSpace(string(b))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestE2E_BasicHTTP verifies that a plain HTTP request is forwarded to the
// backend and the response body arrives intact.
func TestE2E_BasicHTTP(t *testing.T) {
	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello from alpha")) //nolint:errcheck
	}))
	t.Cleanup(be.Close)

	proxy := newProxy(t, "127.0.0.1", false)
	resp := get(t, proxy, domain(backendPort(be), ""), "/")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if got := body(t, resp); got != "hello from alpha" {
		t.Fatalf("body: want %q, got %q", "hello from alpha", got)
	}
}

// TestE2E_MultipleBackends starts three backends each returning a distinct
// string, then verifies that the proxy routes each domain to the correct one.
func TestE2E_MultipleBackends(t *testing.T) {
	type backend struct {
		label string
		srv   *httptest.Server
	}

	backends := []backend{
		{label: "backend-apple"},
		{label: "backend-banana"},
		{label: "backend-cherry"},
	}
	for i := range backends {
		lbl := backends[i].label
		backends[i].srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(lbl)) //nolint:errcheck
		}))
		t.Cleanup(backends[i].srv.Close)
	}

	proxy := newProxy(t, "127.0.0.1", false)

	for _, b := range backends {
		resp := get(t, proxy, domain(backendPort(b.srv), ""), "/")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("[%s] status: want 200, got %d", b.label, resp.StatusCode)
			continue
		}
		if got := body(t, resp); got != b.label {
			t.Errorf("[%s] body: want %q, got %q", b.label, b.label, got)
		}
	}
}

// TestE2E_InvalidDomain_Returns400 checks that a Host header that doesn't
// match the configured suffix returns 400 Bad Request.
func TestE2E_InvalidDomain_Returns400(t *testing.T) {
	proxy := newProxy(t, "127.0.0.1", false)

	cases := []string{
		"not-a-valid-domain.example.com",
		"hello.proxy.test",          // no port/IP
		"3030.10-167-100.proxy.test", // only 3 octets
	}
	for _, host := range cases {
		t.Run(host, func(t *testing.T) {
			resp := get(t, proxy, host, "/")
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("want 400, got %d", resp.StatusCode)
			}
		})
	}
}

// TestE2E_PathAndQueryPassthrough verifies that the path and query string
// are forwarded to the backend unchanged.
func TestE2E_PathAndQueryPassthrough(t *testing.T) {
	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.RequestURI())) //nolint:errcheck
	}))
	t.Cleanup(be.Close)

	proxy := newProxy(t, "127.0.0.1", false)

	cases := []string{
		"/",
		"/some/deep/path",
		"/items?id=42&sort=desc",
		"/a/b?x=1&y=2&z=3",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			resp := get(t, proxy, domain(backendPort(be), ""), path)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status: want 200, got %d", resp.StatusCode)
			}
			if got := body(t, resp); got != path {
				t.Errorf("want %q, got %q", path, got)
			}
		})
	}
}

// TestE2E_ResponseHeaderPassthrough verifies that a custom response header
// set by the backend is forwarded to the client.
func TestE2E_ResponseHeaderPassthrough(t *testing.T) {
	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Id", "node-7")
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	t.Cleanup(be.Close)

	proxy := newProxy(t, "127.0.0.1", false)
	resp := get(t, proxy, domain(backendPort(be), ""), "/")
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Backend-Id"); got != "node-7" {
		t.Errorf("X-Backend-Id: want %q, got %q", "node-7", got)
	}
}

// TestE2E_HTTPSBackend starts a TLS-terminating backend (httptest.NewTLSServer),
// uses the "ssl" domain prefix so the proxy connects over HTTPS, and enables
// InsecureBackend to accept the self-signed test certificate.
func TestE2E_HTTPSBackend(t *testing.T) {
	be := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secure backend")) //nolint:errcheck
	}))
	t.Cleanup(be.Close)

	proxy := newProxy(t, "127.0.0.1", true /* InsecureBackend */)
	resp := get(t, proxy, domain(backendPort(be), "ssl"), "/")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if got := body(t, resp); got != "secure backend" {
		t.Fatalf("body: want %q, got %q", "secure backend", got)
	}
}

// TestE2E_TLSPrefixVariants verifies that "ssl", "tls", and "https" prefixes
// all trigger HTTPS mode against the backend.
func TestE2E_TLSPrefixVariants(t *testing.T) {
	be := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tls ok")) //nolint:errcheck
	}))
	t.Cleanup(be.Close)

	proxy := newProxy(t, "127.0.0.1", true)
	port := backendPort(be)

	for _, proto := range []string{"ssl", "tls", "https"} {
		t.Run(proto, func(t *testing.T) {
			resp := get(t, proxy, domain(port, proto), "/")
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status: want 200, got %d", resp.StatusCode)
			}
			if got := body(t, resp); got != "tls ok" {
				t.Errorf("body: want %q, got %q", "tls ok", got)
			}
		})
	}
}

// TestE2E_SeparatorVariants verifies that dots, dashes, and underscores are
// all valid separators between the port and IP octets.
func TestE2E_SeparatorVariants(t *testing.T) {
	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("sep ok")) //nolint:errcheck
	}))
	t.Cleanup(be.Close)

	proxy := newProxy(t, "127.0.0.1", false)
	port := strconv.Itoa(backendPort(be))

	cases := []struct {
		name string
		host string
	}{
		{"dashes", port + ".10-0-0-1." + e2eSuffix},
		{"dots", port + ".10.0.0.1." + e2eSuffix},
		{"underscores", port + ".10_0_0_1." + e2eSuffix},
		{"mixed", port + ".10.0-0_1." + e2eSuffix},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := get(t, proxy, tc.host, "/")
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status: want 200, got %d", resp.StatusCode)
			}
			if got := body(t, resp); got != "sep ok" {
				t.Errorf("body: want %q, got %q", "sep ok", got)
			}
		})
	}
}

// TestE2E_BackendStatusCodesPassthrough verifies that non-200 status codes
// from the backend are forwarded unchanged.
func TestE2E_BackendStatusCodesPassthrough(t *testing.T) {
	cases := []struct {
		name string
		code int
	}{
		{"404", http.StatusNotFound},
		{"500", http.StatusInternalServerError},
		{"201", http.StatusCreated},
	}

	proxy := newProxy(t, "127.0.0.1", false)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := tc.code
			be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			t.Cleanup(be.Close)

			resp := get(t, proxy, domain(backendPort(be), ""), "/")
			defer resp.Body.Close()
			if resp.StatusCode != code {
				t.Errorf("want %d, got %d", code, resp.StatusCode)
			}
		})
	}
}
