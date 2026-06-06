# haberdasher

Lightweight reverse proxy manager. Single container, automatic HTTPS via Let's Encrypt (Caddy), React UI.

## Quick start (Docker)

```bash
docker build -t haberdasher:latest .

docker run -d \
  --name haberdasher \
  -p 80:80 \
  -p 443:443 \
  -p 8080:8080 \
  -v haberdasher-data:/data \
  --restart unless-stopped \
  haberdasher:latest
```

Open http://localhost:8080 — you'll be walked through the setup wizard.

## Proxmox LXC (bare metal, no Docker)

### 1. Create LXC

Use Debian 12 or Ubuntu 24.04. Give it a static IP. Unprivileged is fine.

### 2. Install Caddy

```bash
apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
apt update && apt install caddy
systemctl stop caddy && systemctl disable caddy  # haberdasher manages caddy itself
```

### 3. Deploy haberdasher

```bash
# On your build machine
make deploy-lxc

# Copy to LXC
scp bin/haberdasher-linux root@<lxc-ip>:/usr/local/bin/haberdasher
chmod +x /usr/local/bin/haberdasher

# On the LXC
useradd -r -s /bin/false -d /var/lib/haberdasher haberdasher
mkdir -p /var/lib/haberdasher
chown haberdasher:haberdasher /var/lib/haberdasher

cp haberdasher.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now haberdasher
```

Open http://<lxc-ip>:8080 for the UI.

## Stack

- **Caddy** — reverse proxy engine, automatic ACME/Let's Encrypt, zero-downtime reloads
- **Go + Chi** — REST API backend, SQLite config storage
- **React + Vite** — single-page UI, embedded into the binary at build time
- **SQLite** — all config stored in a single file at `$HABERDASHER_DATA/haberdasher.db`

## Ports

| Port | Purpose |
|------|---------|
| 80   | HTTP (proxied traffic + ACME HTTP-01 challenge) |
| 443  | HTTPS (proxied traffic) |
| 8080 | Haberdasher management UI |

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HABERDASHER_DATA` | `/data` | Data directory (SQLite db + Caddy certs) |
| `HABERDASHER_LISTEN` | `:8080` | Management UI listen address |

## Metrics

Per-request metrics are pushed to configured destinations on every proxied request.

Metric naming:

```
# StatsD / Graphite
{prefix}.proxy.{host_slug}.requests
{prefix}.proxy.{host_slug}.bytes_in
{prefix}.proxy.{host_slug}.bytes_out
{prefix}.proxy.{host_slug}.response_ms
{prefix}.proxy.{host_slug}.status.2xx

# InfluxDB (line protocol)
measurement: {prefix}_proxy_requests
tags: host, host_slug, status_class, method, alias (if set)
```

Set a **metrics alias** on any proxy host to use a short name instead of the sanitized domain in metric paths.

## Features

- Proxy hosts with per-host enable/disable toggle
- Automatic HTTPS via Let's Encrypt (HTTP-01 and DNS-01)
- WebSocket support
- HTTP→HTTPS redirect
- IP allow/deny lists per host
- HTTP basic auth per host
- Metric push to StatsD, Graphite, InfluxDB v1/v2
- Configurable metric prefix per destination
- Per-request metrics on every proxied connection
- First-run wizard, JWT auth, optional TOTP 2FA
- Audit log
