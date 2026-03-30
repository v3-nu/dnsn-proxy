package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"regexp"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule((*DNSNProxyHandler)(nil))
}

// DNSNProxyHandler is a Caddy middleware that parses the request host to
// determine the backend port (and optional TLS) and reverse-proxies to it.
type DNSNProxyHandler struct {
	Suffix          string `json:"suffix"`
	Backend         string `json:"backend"`
	InsecureBackend bool   `json:"insecure_backend,omitempty"`

	re     *regexp.Regexp
	logger *zap.Logger
}

// CaddyModule returns the Caddy module information.
func (*DNSNProxyHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.dnsn_proxy",
		New: func() caddy.Module { return new(DNSNProxyHandler) },
	}
}

// Provision sets up the handler.
func (h *DNSNProxyHandler) Provision(ctx caddy.Context) error {
	h.logger = ctx.Logger()
	h.re = buildRegex(h.Suffix)
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (h *DNSNProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Strip port from Host header if present.
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	result, ok := ParseDomain(h.re, host)
	if !ok {
		http.Error(w, "domain not recognised", http.StatusBadRequest)
		return nil
	}

	scheme := "http"
	if result.UseSSL {
		scheme = "https"
	}

	target := fmt.Sprintf("%s://%s:%d", scheme, h.Backend, result.Port)

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = scheme
			req.URL.Host = fmt.Sprintf("%s:%d", h.Backend, result.Port)
			req.Host = req.URL.Host
		},
	}

	if h.InsecureBackend {
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	h.logger.Debug("proxying request", zap.String("target", target), zap.String("host", host))
	proxy.ServeHTTP(w, r)
	return nil
}

// Interface guards.
var (
	_ caddy.Module             = (*DNSNProxyHandler)(nil)
	_ caddy.Provisioner        = (*DNSNProxyHandler)(nil)
	_ caddyhttp.MiddlewareHandler = (*DNSNProxyHandler)(nil)
)
