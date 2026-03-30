#!/bin/sh
set -e

# Create the data directory (certmagic cert storage).
mkdir -p /var/lib/dnsn-proxy /var/log/dnsn-proxy
chown dnsn-proxy:dnsn-proxy /var/lib/dnsn-proxy 2>/dev/null || true
chmod 0750 /var/lib/dnsn-proxy

rc-update add dnsn-proxy default

echo ""
echo "dnsn-proxy installed."
echo "  1. Edit /etc/conf.d/dnsn-proxy to set your domain suffix and ACME email."
echo "  2. Run: rc-service dnsn-proxy start"
echo ""
