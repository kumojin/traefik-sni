# Manual Testing

Manual tests validate the traefik-sni plugin on a real server (e.g., a DigitalOcean droplet). Two phases:

1. **Vulnerable** -- prove domain fronting works without the plugin.
2. **Protected** -- prove the plugin blocks it.

## Test Documents

1. [Testing the vulnerability](manual-test-vulnerability.md) -- prove domain fronting works without the plugin
2. [Testing with local plugin](manual-test-local-plugin.md) -- prove the plugin blocks domain fronting
3. [Testing from Plugin Catalog](manual-test-catalog.md) -- once published to the catalog

## Prerequisites

- A server (DigitalOcean droplet, Debian 12+ recommended)
- Docker installed
- A domain with two DNS A records pointing to the server:
  - `legit.example.com` -> `<SERVER_IP>`
  - `victim.example.com` -> `<SERVER_IP>`

> Replace every occurrence of `example.com` with your actual domain and `<SERVER_IP>` with your server's public IP throughout all documents.

## Setup

### Create working directory

```bash
mkdir -p ~/traefik-test && cd ~/traefik-test
```

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

### Create Docker network and start backends

```bash
docker network create sni-test

docker run -d --name legit  --network sni-test traefik/whoami --name legit
docker run -d --name victim --network sni-test traefik/whoami --name victim
```

## Cleanup

After testing is complete:

```bash
docker rm -f traefik legit victim
docker network rm sni-test
```

Destroy the droplet when done.
