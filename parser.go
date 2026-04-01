package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParseResult holds the parsed information from a domain name.
type ParseResult struct {
	Port   int
	UseSSL bool
}

// buildRegex constructs the matching regexp for the given suffix.
// works with WildIP (e.g. 10.10.10.1.sslip.io or 10-11-12-13.dnsn.eu) and ipw4-type addresses (e.g. cat.nil.nil.cat.ipw4.com)
func buildRegex(suffix string) *regexp.Regexp {
	escaped := regexp.QuoteMeta(suffix)
	pattern := fmt.Sprintf(
		`(?i)^([a-zA-Z0-9]+[\._-])?(?:(ssl|tls|https))?(\d{1,5})([.\-_](\d{1,3}|[a-zA-Z]{3})[.\-_](\d{1,3}|[a-zA-Z]{3})[.\-_](\d{1,3}|[a-zA-Z]{3})[.\-_](\d{1,3}|[a-zA-Z]{3}))?[.\-_]%s$`,
		escaped,
	)
	return regexp.MustCompile(pattern)
}

// ParseDomain parses an FQDN using the pre-built regex.
// Returns the ParseResult and true on success, or zero value and false on failure.
func ParseDomain(re *regexp.Regexp, fqdn string) (ParseResult, bool) {
	// Strip trailing dot (FQDN form)
	host := strings.TrimSuffix(fqdn, ".")

	m := re.FindStringSubmatch(host)
	if m == nil {
		return ParseResult{}, false
	}

	proto := strings.ToLower(m[2])
	useSSL := proto == "ssl" || proto == "tls" || proto == "https"

	port, err := strconv.Atoi(m[3])
	if err != nil || port < 1 || port > 65535 {
		return ParseResult{}, false
	}

	return ParseResult{Port: port, UseSSL: useSSL}, true
}
