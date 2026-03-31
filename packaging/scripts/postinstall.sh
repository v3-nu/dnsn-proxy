#!/bin/sh
set -e

# Ensure the data directory exists with correct ownership.
# certmagic stores TLS certificates under $HOME/.local/share/caddy.
install -d -m 0750 -o dnsn-proxy -g dnsn-proxy /var/lib/dnsn-proxy

systemctl daemon-reload
systemctl enable dnsn-proxy.service

echo ""
echo "dnsn-proxy installed."
echo "  1. Edit /etc/dnsn-proxy/config.yaml to set your domain suffix and ACME email."
echo "  2. Run: systemctl start dnsn-proxy"
echo ""
