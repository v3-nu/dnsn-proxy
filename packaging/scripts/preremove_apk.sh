#!/bin/sh
set -e

rc-service dnsn-proxy stop    2>/dev/null || true
rc-update  del    dnsn-proxy  2>/dev/null || true
