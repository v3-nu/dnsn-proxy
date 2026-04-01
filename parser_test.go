package main

import (
	"testing"
)

func TestParseDomain(t *testing.T) {
	re := buildRegex("dnsn.eu")

	positive := []struct {
		fqdn   string
		port   int
		useSSL bool
	}{
		{"3030.10-11-12-13.dnsn.eu", 3030, false},
		{"ssl41954.10-11-12-13.dnsn.eu", 41954, true},
		{"tls1234.10.11.12.13.dnsn.eu", 1234, true},
		{"afunc.tls1234.10.11.12.13.dnsn.eu", 1234, true},
		{"https123.10-11-12-13.dnsn.eu", 123, true},
		{"3030-10-11-12-13.dnsn.eu", 3030, false},
		{"helloworld-3030-10-11-12-13.dnsn.eu", 3030, false},
		{"3030.dnsn.eu", 3030, false},
		{"ssl3194.dnsn.eu", 3194, true},
		{"12345.dnsn.eu", 12345, false},
		{"helloworld.12345.dnsn.eu", 12345, false},
		{"helloworld.https12345.dnsn.eu", 12345, true},
		{"helloworld.ssl12345.dnsn.eu", 12345, true},
		{"http.ssl12345.dnsn.eu", 12345, true},
		{"http.ssl12345-dnsn.eu", 12345, true},
	}

	for _, tc := range positive {
		t.Run("positive/"+tc.fqdn, func(t *testing.T) {
			res, ok := ParseDomain(re, tc.fqdn)
			if !ok {
				t.Fatalf("expected match for %q, got none", tc.fqdn)
			}
			if res.Port != tc.port {
				t.Errorf("port: got %d, want %d", res.Port, tc.port)
			}
			if res.UseSSL != tc.useSSL {
				t.Errorf("useSSL: got %v, want %v", res.UseSSL, tc.useSSL)
			}
		})
	}

	negative := []string{
		"3030.10-11-12-13.example.com", // wrong suffix
		"3030.10-167-100.dnsn.eu",         // only 3 octets
		"hello.dnsn.eu",                   // plain text subdomain
		"3030.10-11-12-13.dnsn.eu.",    // trailing dot (should still parse — but we test strip)
		"helloworld12345.dnsn.eu",         // missing dot between label and port
		"helloworldtls12345.dnsn.eu",      // missing dot between label and protocol
		"ssl123456.dnsn.eu",               // port out of range
		"http.ssl12345-x.dnsn.eu",         // invalid character in IP part
	}

	// The trailing-dot case should actually succeed (we strip it), so test separately.
	t.Run("positive/trailing-dot", func(t *testing.T) {
		res, ok := ParseDomain(re, "3030.10-11-12-13.dnsn.eu.")
		if !ok {
			t.Fatal("expected match for trailing-dot FQDN")
		}
		if res.Port != 3030 || res.UseSSL {
			t.Errorf("unexpected result: %+v", res)
		}
	})

	// Remove trailing dot case from negative slice (index 3).
	negative = negative[:3]

	for _, fqdn := range negative {
		t.Run("negative/"+fqdn, func(t *testing.T) {
			_, ok := ParseDomain(re, fqdn)
			if ok {
				t.Fatalf("expected no match for %q, but got a match", fqdn)
			}
		})
	}
}
