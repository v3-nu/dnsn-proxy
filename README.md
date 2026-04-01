# dnsn-proxy

A reverse proxy that terminates HTTPS on-demand using wildcard DNS services such as [sslip.io](https://sslip.io) or [dnsn.eu](https://dnsn.eu). It decodes the target port (and optional TLS mode) from the request hostname and forwards the request to a backend server — no manual certificate provisioning required.

## How it works

A client connects to a hostname that encodes the backend port and your server's IP address, for example:

```
3000.10-11-12-13.dnsn.eu
```

dnsn-proxy:
1. Receives the HTTPS request (Caddy obtains a certificate on-demand via ACME).
2. Parses the hostname to extract the port number (`3000`) and whether TLS should be used toward the backend.
3. Reverse-proxies the request to `backend:3000`.

### Hostname format

```
[ssl|tls|https]<port>.<ip-octets>.<suffix>
```

| Part | Description |
|---|---|
| `ssl` / `tls` / `https` | Optional prefix — proxy to backend over HTTPS instead of HTTP |
| `<port>` | Destination port on the backend (1–65535) |
| `<ip-octets>` | Four octets of an IP address separated by `.`, `-`, or `_` |
| `<suffix>` | Your configured DNS suffix (e.g. `dnsn.eu`) |

**Examples**

| Hostname | Backend connection |
|---|---|
| `3000.10-11-12-13.dnsn.eu` | `http://localhost:3000` |
| `ssl8443.10.11.12.13.dnsn.eu` | `https://localhost:8443` |
| `tls1234.10-11-12-13.dnsn.eu` | `https://localhost:1234` |

HTTP requests on port 80 are redirected to HTTPS (308).

You can also use a regular domain name instead of an IP octet-type domain, for example, using the suffix myapp.example.com and pointing `*.myapp.example.com` to your server's IP:

**Examples**
| Hostname | Backend connection |
|---|---|
| `3000.myapp.example.com` | `http://localhost:3000` |
| `ssl8443.myapp.example.com` | `https://localhost:8443` |
| `tls1234.myapp.example.com` | `https://localhost:1234` |

There is also support for a label prefix which has no routing implications but allows for multiple hostnames on the same destination port, for example:

**Examples**
| Hostname | Backend connection |
|---|---|
| `app1-3000.myapp.example.com` | `http://localhost:3000` |
| `app2-ssl3000.myapp.example.com` | `https://localhost:3000` |
| `app1-tls3031.10-11-12-13.dnsn.eu` | `https://localhost:3031` |
| `app1_tls3031.10-11-12-13.dnsn.eu` | `https://localhost:3031` |
| `app1.tls3031.10-11-12-13.dnsn.eu` | `https://localhost:3031` |

## Configuration

Copy `config.example.yaml` to `/etc/dnsn-proxy/config.yaml` and adjust as needed.

```yaml
suffix: dnsn.eu
backend: localhost
acme_email: admin@example.com
```

### All options

| Key | Default | Description |
|---|---|---|
| `suffix` | — | **Required.** Wildcard DNS suffix to match (e.g. `dnsn.eu`) |
| `backend` | — | **Required.** Hostname or IP to proxy requests to |
| `acme_email` | — | **Required.** Email for ACME certificate registration |
| `acme_ca` | Let's Encrypt production | ACME CA directory URL |
| `http_port` | `80` | Port to listen on for HTTP (redirects to HTTPS) |
| `https_port` | `443` | Port to listen on for HTTPS |
| `insecure_backend` | `false` | Skip TLS verification when proxying to the backend |
| `ask_port` | `19999` | Internal port for the ACME on-demand permission server |
| `additional_hosts` | — | List of fixed hostname → backend mappings (see below) |

#### `additional_hosts`

Routes a fixed hostname to a specific backend, bypassing the wildcard DNS pattern. Matched before the dnsn domain parser.

```yaml
additional_hosts:
  - hostname: grafana.example.com
    backend: 192.168.1.20
    port: 3000
    ssl: false
  - hostname: vault.example.com
    backend: 192.168.1.30
    port: 8200
    ssl: true
```

## Installation

### From source

```sh
go build -o dnsn-proxy .
sudo install -m 755 dnsn-proxy /usr/local/bin/dnsn-proxy
sudo mkdir -p /etc/dnsn-proxy
sudo cp config.example.yaml /etc/dnsn-proxy/config.yaml
```

### Running

```sh
dnsn-proxy -config /etc/dnsn-proxy/config.yaml
```

The `-config` flag defaults to `/etc/dnsn-proxy/config.yaml`.

### systemd

A unit file is provided in `packaging/dnsn-proxy.service`. Install it with:

```sh
sudo cp packaging/dnsn-proxy.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now dnsn-proxy
```

### OpenRC (Alpine / Gentoo)

```sh
sudo cp packaging/dnsn-proxy.openrc /etc/init.d/dnsn-proxy
sudo cp packaging/dnsn-proxy.confd  /etc/conf.d/dnsn-proxy
sudo rc-update add dnsn-proxy default
sudo rc-service dnsn-proxy start
```

## Prerequisites

- The machine must be reachable on ports 80 and 443 from the internet so that Let's Encrypt can issue certificates.
- Your DNS suffix must point wildcard DNS entries to this machine's IP (e.g. `*.dnsn.eu → <your IP>`).
- Port `ask_port` (default 19999) must be free on localhost.

## Development

```sh
go test ./...
```
