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
	"gopkg.in/yaml.v3"

	// Register built-in Caddy modules.
	_ "github.com/caddyserver/caddy/v2/modules/caddyhttp"
	_ "github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	_ "github.com/caddyserver/caddy/v2/modules/caddypki"
	_ "github.com/caddyserver/caddy/v2/modules/caddytls"
)

// AdditionalHost maps a fixed hostname to a specific backend server and port.
type AdditionalHost struct {
	Hostname string `yaml:"hostname" json:"hostname"`
	Backend  string `yaml:"backend"  json:"backend"`
	Port     int    `yaml:"port"     json:"port"`
	SSL      bool   `yaml:"ssl"      json:"ssl,omitempty"`
}

// Config holds all runtime configuration loaded from the YAML config file.
type Config struct {
	Suffix          string           `yaml:"suffix"`
	Backend         string           `yaml:"backend"`
	AcmeCA          string           `yaml:"acme_ca"`
	AcmeEmail       string           `yaml:"acme_email"`
	AcmeEabKid      string           `yaml:"acme_eab_kid"`
	AcmeEabHmacKey  string           `yaml:"acme_eab_hmac_key"`
	TLSCert         string           `yaml:"tls_cert"`
	TLSKey          string           `yaml:"tls_key"`
	HTTPPort        int              `yaml:"http_port"`
	HTTPSPort       int              `yaml:"https_port"`
	InsecureBackend bool             `yaml:"insecure_backend"`
	AskPort         int              `yaml:"ask_port"`
	AdditionalHosts []AdditionalHost `yaml:"additional_hosts"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.AcmeCA == "" {
		cfg.AcmeCA = "https://acme-v02.api.letsencrypt.org/directory"
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 80
	}
	if cfg.HTTPSPort == 0 {
		cfg.HTTPSPort = 443
	}
	if cfg.AskPort == 0 {
		cfg.AskPort = 19999
	}
	return &cfg, nil
}

func main() {
	configPath := flag.String("config", "/etc/dnsn-proxy/config.yaml", "Path to the YAML configuration file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Start the ask server before Caddy so it is ready when Caddy needs it.
	askSrv := startAskServer(cfg.AskPort, cfg.Suffix, cfg.AdditionalHosts)

	caddyCfg, err := buildConfig(cfg)
	if err != nil {
		log.Fatalf("build config: %v", err)
	}

	if err := caddy.Load(caddyCfg, true); err != nil {
		log.Fatalf("caddy load: %v", err)
	}

	log.Printf("dnsn-proxy running (suffix=%s backend=%s https=:%d http=:%d)",
		cfg.Suffix, cfg.Backend, cfg.HTTPSPort, cfg.HTTPPort)

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
// Caddy calls GET /?domain=<fqdn>; we return 200 if the domain matches the
// suffix pattern or is listed in additionalHosts.
func startAskServer(port int, suffix string, additionalHosts []AdditionalHost) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		domain := r.URL.Query().Get("domain")
		re := buildRegex(suffix)
		if _, ok := ParseDomain(re, domain); ok {
			w.WriteHeader(http.StatusOK)
			return
		}
		for _, h := range additionalHosts {
			if h.Hostname == domain {
				w.WriteHeader(http.StatusOK)
				return
			}
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
func buildConfig(cfg *Config) ([]byte, error) {
	c := map[string]interface{}{
		"admin": map[string]interface{}{
			"disabled": true,
		},
		"apps": map[string]interface{}{
			"http": map[string]interface{}{
				"servers": map[string]interface{}{
					"https_srv": map[string]interface{}{
						"listen": []string{fmt.Sprintf("0.0.0.0:%d", cfg.HTTPSPort)},
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
										"suffix":           cfg.Suffix,
										"backend":          cfg.Backend,
										"insecure_backend": cfg.InsecureBackend,
										"additional_hosts": cfg.AdditionalHosts,
									},
								},
							},
						},
					},
					"http_srv": map[string]interface{}{
						"listen": []string{fmt.Sprintf("0.0.0.0:%d", cfg.HTTPPort)},
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
			"tls": buildTLSConfig(cfg),
		},
	}

	return json.MarshalIndent(c, "", "  ")
}

func buildTLSConfig(cfg *Config) map[string]interface{} {
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		return map[string]interface{}{
			"certificates": map[string]interface{}{
				"load_files": []interface{}{
					map[string]interface{}{
						"certificate": cfg.TLSCert,
						"key":         cfg.TLSKey,
					},
				},
			},
		}
	}

	return map[string]interface{}{
		"automation": map[string]interface{}{
			"on_demand": map[string]interface{}{
				"permission": map[string]interface{}{
					"module":   "http",
					"endpoint": fmt.Sprintf("http://127.0.0.1:%d/", cfg.AskPort),
				},
			},
			"policies": []interface{}{
				map[string]interface{}{
					"on_demand": true,
					"issuers": []interface{}{
						map[string]interface{}{
							"module": "acme",
							"ca":     cfg.AcmeCA,
							"email":  cfg.AcmeEmail,
							// "external_account": map[string]interface{}{
							// 	"key_id":  cfg.AcmeEabKid,
							// 	"mac_key": cfg.AcmeEabHmacKey,
							// },
							// "challenges": map[string]interface{}{
							// 	"dns": map[string]interface{}{
							// 		"provider": map[string]string{
							// 			"name": "noop",
							// 		},
							// 	},
							// },
						},
					},
				},
			},
		},
	}
}
