#!/bin/sh
set -e

# Alpine uses busybox addgroup/adduser, not shadow-utils.
addgroup -S dnsn-proxy 2>/dev/null || true
adduser -S -G dnsn-proxy -H -h /var/lib/dnsn-proxy -s /sbin/nologin dnsn-proxy 2>/dev/null || true
