#!/bin/sh
set -e

# Create the service account used by the systemd unit.
# groupadd/useradd are provided by shadow-utils on Debian and RHEL.
if ! getent group dnsn-proxy > /dev/null 2>&1; then
    groupadd --system dnsn-proxy
fi

if ! getent passwd dnsn-proxy > /dev/null 2>&1; then
    useradd \
        --system \
        --gid dnsn-proxy \
        --home-dir /var/lib/dnsn-proxy \
        --no-create-home \
        --shell /usr/sbin/nologin \
        --comment "dnsn-proxy service account" \
        dnsn-proxy
fi
