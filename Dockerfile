FROM gcr.io/distroless/static-debian12

COPY dnsn-proxy /dnsn-proxy

# 80  — HTTP (redirects to HTTPS)
# 443 — HTTPS + on-demand TLS
EXPOSE 80 443

ENTRYPOINT ["/dnsn-proxy"]
