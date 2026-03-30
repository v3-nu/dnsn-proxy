package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caddyserver/caddy/v2"

	// Register built-in Caddy modules.
	_ "github.com/caddyserver/caddy/v2/modules/caddyhttp"
	_ "github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	_ "github.com/caddyserver/caddy/v2/modules/caddypki"
	_ "github.com/caddyserver/caddy/v2/modules/caddytls"
)

func main() {
	suffix := flag.String("suffix", "dnsn.eu", "Domain suffix handled by this proxy")
	backend := flag.String("backend", "localhost", "Backend host to proxy to")
	acmeCA := flag.String("acme-ca", "https://acme-v02.api.letsencrypt.org/directory", "ACME CA directory URL")
	acmeEmail := flag.String("acme-email", "", "Email for ACME registration")
	httpPort := flag.Int("http-port", 80, "HTTP listen port")
	httpsPort := flag.Int("https-port", 443, "HTTPS listen port")
	insecureBackend := flag.Bool("insecure-backend", false, "Skip TLS verification for backend")
	askPort := flag.Int("ask-port", 19999, "Port for the on-demand TLS ask server")
	flag.Parse()

	// Start the ask server before Caddy so it is ready when Caddy needs it.
	askSrv := startAskServer(*askPort, *suffix)

	cfg, err := buildConfig(*suffix, *backend, *acmeCA, *acmeEmail, *httpPort, *httpsPort, *insecureBackend, *askPort)
	if err != nil {
		log.Fatalf("build config: %v", err)
	}

	if err := caddy.Load(cfg, true); err != nil {
		log.Fatalf("caddy load: %v", err)
	}

	log.Printf("dnsn-proxy running (suffix=%s backend=%s https=:%d http=:%d)", *suffix, *backend, *httpsPort, *httpPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down…")

	if err := caddy.Stop(); err != nil {
		log.Printf("caddy stop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := askSrv.Shutdown(ctx); err != nil {
		log.Printf("ask server shutdown: %v", err)
	}
}

// startAskServer runs a tiny HTTP server on 127.0.0.1:{port}.
// Caddy calls GET /?domain=<fqdn>; we return 200 if it ends with the suffix.
func startAskServer(port int, suffix string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		domain := r.URL.Query().Get("domain")
		re := buildRegex(suffix)
		if _, ok := ParseDomain(re, domain); ok {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not allowed", http.StatusForbidden)
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ask server: %v", err)
		}
	}()

	return srv
}

// buildConfig constructs the Caddy JSON configuration.
func buildConfig(suffix, backend, acmeCA, acmeEmail string, httpPort, httpsPort int, insecureBackend bool, askPort int) ([]byte, error) {
	cfg := map[string]interface{}{
		"admin": map[string]interface{}{
			"disabled": true,
		},
		"apps": map[string]interface{}{
			"http": map[string]interface{}{
				"servers": map[string]interface{}{
					"https_srv": map[string]interface{}{
						"listen": []string{fmt.Sprintf(":%d", httpsPort)},
						"automatic_https": map[string]interface{}{
							"disable": true,
						},
						"tls_connection_policies": []interface{}{
							map[string]interface{}{},
						},
						"routes": []interface{}{
							map[string]interface{}{
								"handle": []interface{}{
									map[string]interface{}{
										"handler":          "dnsn_proxy",
										"suffix":           suffix,
										"backend":          backend,
										"insecure_backend": insecureBackend,
									},
								},
							},
						},
					},
					"http_srv": map[string]interface{}{
						"listen": []string{fmt.Sprintf(":%d", httpPort)},
						"automatic_https": map[string]interface{}{
							"disable": true,
						},
						"routes": []interface{}{
							map[string]interface{}{
								"handle": []interface{}{
									map[string]interface{}{
										"handler":     "static_response",
										"status_code": 308,
										"headers": map[string]interface{}{
											"Location": []string{"https://{http.request.host}{http.request.uri}"},
										},
									},
								},
							},
						},
					},
				},
			},
			"tls": map[string]interface{}{
				"automation": map[string]interface{}{
					"on_demand": map[string]interface{}{
						"permission": map[string]interface{}{
							"module":   "http",
							"endpoint": fmt.Sprintf("http://127.0.0.1:%d/", askPort),
						},
					},
					"policies": []interface{}{
						map[string]interface{}{
							"on_demand": true,
							"issuers": []interface{}{
								map[string]interface{}{
									"module": "acme",
									"ca":     acmeCA,
									"email":  acmeEmail,
								},
							},
						},
					},
				},
			},
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}
