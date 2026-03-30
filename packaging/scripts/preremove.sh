#!/bin/sh
set -e

systemctl stop    dnsn-proxy.service 2>/dev/null || true
systemctl disable dnsn-proxy.service 2>/dev/null || true
