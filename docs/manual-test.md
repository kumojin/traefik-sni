# Manual Testing

Manual tests validate the traefik-sni-host-check plugin on a real server (e.g., a DigitalOcean droplet). Two phases:

1. **Vulnerable** -- prove domain fronting works without the plugin.
2. **Protected** -- prove the plugin blocks it.

## Prerequisites

- A server (DigitalOcean droplet, Debian 12+ recommended)
- A domain with two DNS A records pointing to the server:
  - `legit.example.com` -> `<SERVER_IP>`
  - `victim.example.com` -> `<SERVER_IP>`

> Replace every occurrence of `example.com` with your actual domain and `<SERVER_IP>` with your server's public IP throughout all documents.

## Setup

### Install Traefik

If Traefik is not already installed on the server, download the binary:

```bash
curl -sL https://github.com/traefik/traefik/releases/download/v3.3.6/traefik_v3.3.6_linux_amd64.tar.gz \
  | tar xz -C /usr/local/bin traefik
traefik version
```

> See [Traefik releases](https://github.com/traefik/traefik/releases) for the latest v3.3+ version.

### Create working directory

```bash
mkdir -p ~/traefik-test && cd ~/traefik-test
```

> **Important:** All subsequent commands and config files assume you are in `~/traefik-test`. Traefik resolves relative paths (e.g., `certs/`, `./dynamic`) from the current working directory, so always start Traefik from this directory.

### Generate a self-signed wildcard certificate

A wildcard cert models real shared infrastructure (CDN, cloud proxy) where one cert covers all tenant subdomains. This makes domain fronting invisible at the TLS layer.

```bash
mkdir -p certs
openssl req -x509 -newkey rsa:2048 \
  -keyout certs/key.pem \
  -out certs/cert.pem \
  -days 30 -nodes \
  -subj "/CN=*.example.com" \
  -addext "subjectAltName=DNS:*.example.com"
```

### Download and start backends

Download the [whoami](https://github.com/traefik/whoami) binary (~2.6 MB):

```bash
curl -sL https://github.com/traefik/whoami/releases/download/v1.11.0/whoami_v1.11.0_linux_amd64.tar.gz \
  | tar xz -C /usr/local/bin whoami
```

Start two instances as background processes:

```bash
whoami --port 8001 --name legit &
whoami --port 8002 --name victim &
```

Verify they are running:

```bash
curl -s http://127.0.0.1:8001/ | grep "Name: legit"
curl -s http://127.0.0.1:8002/ | grep "Name: victim"
```

## Next Steps

Start with the vulnerability test to confirm domain fronting works without the plugin, then test the protection:

1. **[Testing the vulnerability](manual-test-vulnerable.md)** -- start here to prove domain fronting works without the plugin
2. **[Testing with the plugin](manual-test-protected.md)** -- prove the plugin blocks domain fronting (local plugin or Plugin Catalog)

## Cleanup

After testing is complete:

```bash
# Stop backends
pkill -f "whoami --port 800"

# Stop Traefik (if still running)
pkill traefik
```

Destroy the droplet when done.
