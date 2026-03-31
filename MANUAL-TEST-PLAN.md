# Manual Test Plan: traefik-sni plugin on DigitalOcean

Test the plugin on a real server to validate it blocks domain fronting
in a production-like environment.

## Overview

Two phases, same approach as the automated e2e tests:

1. **Phase 1 (Vulnerable):** Traefik with no plugin. Prove domain
   fronting works — a request with `SNI=legit` but `Host: victim`
   reaches the victim backend.
2. **Phase 2 (Protected):** Enable the traefik-sni plugin via the
   GitHub provider. Same attack now returns 421 Misdirected Request.

## Prerequisites

- A DigitalOcean droplet (Ubuntu 22.04+ recommended)
- Docker installed on the droplet
- Two DNS A records pointing to the droplet's IP, e.g.:
  - `legit.example.com` → `<DROPLET_IP>`
  - `victim.example.com` → `<DROPLET_IP>`
- The GitHub repo **must be public** for Traefik's GitHub plugin
  provider to download it (it fetches a zipball from the GitHub API
  without authentication)

> **Note:** If you don't want to make the repo public yet, see
> [Alternative: local plugin mode](#alternative-local-plugin-mode) at the
> bottom.

## Step 1: Prepare the droplet

SSH into the droplet and install Docker if not already present:

```bash
ssh root@<DROPLET_IP>
curl -fsSL https://get.docker.com | sh
```

Create a working directory:

```bash
mkdir -p ~/traefik-test && cd ~/traefik-test
```

## Step 2: Generate a self-signed wildcard TLS certificate

A wildcard cert models the real-world scenario: shared infrastructure
(CDN, cloud proxy) where one cert covers all tenant subdomains. This
makes domain fronting completely invisible at the TLS layer — the
handshake succeeds cleanly for any subdomain.

```bash
mkdir -p certs
openssl req -x509 -newkey rsa:2048 \
  -keyout certs/key.pem \
  -out certs/cert.pem \
  -days 30 -nodes \
  -subj "/CN=*.example.com" \
  -addext "subjectAltName=DNS:*.example.com"
```

> **Alternative:** Use Let's Encrypt with Traefik's ACME resolver for
> real certs. This works but adds complexity (email, storage, DNS
> propagation delay). Self-signed is sufficient — the plugin checks
> SNI vs Host, not certificate trust.

## Step 3: Create a Docker network and start backends

```bash
docker network create sni-test

docker run -d --name legit  --network sni-test traefik/whoami --name legit
docker run -d --name victim --network sni-test traefik/whoami --name victim
```

## Step 4: Write the Traefik static config

Replace `legit.example.com` and `victim.example.com` with your actual
domains throughout this file and the dynamic configs below.

Create `traefik.yml`:

```yaml
log:
  level: DEBUG

entryPoints:
  websecure:
    address: ":443"

providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
```

> **No plugin yet.** Phase 1 runs without the plugin to prove the
> vulnerability exists.

## Step 5: Write the vulnerable dynamic config

Create `dynamic.yml`:

```yaml
http:
  routers:
    legit:
      rule: "Host(`legit.example.com`)"
      entryPoints:
        - websecure
      service: legit
      tls: {}

    victim:
      rule: "Host(`victim.example.com`)"
      entryPoints:
        - websecure
      service: victim
      tls: {}

  services:
    legit:
      loadBalancer:
        servers:
          - url: "http://legit:80"

    victim:
      loadBalancer:
        servers:
          - url: "http://victim:80"

tls:
  certificates:
    - certFile: /etc/traefik/certs/cert.pem
      keyFile: /etc/traefik/certs/key.pem
```

## Step 6: Start Traefik (vulnerable)

```bash
docker run -d --name traefik --network sni-test \
  -p 443:443 \
  -v $(pwd)/traefik.yml:/etc/traefik/traefik.yml:ro \
  -v $(pwd)/dynamic.yml:/etc/traefik/dynamic/dynamic.yml:ro \
  -v $(pwd)/certs:/etc/traefik/certs:ro \
  traefik:v3.3
```

Wait a few seconds, then verify Traefik is up:

```bash
docker logs traefik
```

## Step 7: Phase 1 — prove domain fronting works

Run these from your **local machine** (not the droplet), or from
anywhere with curl:

### 7a. Normal request (should work)

```bash
curl -sk https://legit.example.com/
```

Expected: response body contains `Name: legit`.

### 7b. Domain fronting attack (should succeed — this is the vulnerability)

```bash
curl -sk --resolve legit.example.com:443:<DROPLET_IP> \
  -H "Host: victim.example.com" \
  https://legit.example.com/
```

What's happening:
- TLS handshake uses SNI = `legit.example.com`
- HTTP Host header says `victim.example.com`
- Traefik routes on Host, so the request reaches the **victim** backend

Expected: response body contains `Name: victim`. **This is the
vulnerability.** The attacker connected to `legit` but reached `victim`.

## Step 8: Stop Traefik

```bash
docker rm -f traefik
```

## Step 9: Enable the plugin (Phase 2)

### 9a. Update the static config

Edit `traefik.yml` to add the plugin via the GitHub provider:

```yaml
log:
  level: DEBUG

entryPoints:
  websecure:
    address: ":443"

experimental:
  plugins:
    traefik-sni:
      moduleName: github.com/kumojin/traefik-sni
      version: v0.1.0

providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
```

> **About the version field:** This must be a git tag on the repo. Create
> one before this step:
> ```bash
> # From your local machine, in the repo:
> git tag v0.1.0
> git push origin v0.1.0
> ```
>
> If you want to try a commit SHA instead of a tag (e.g. `v0.0.0-6d66180`
> as a pseudo-version), it may or may not work — the Traefik docs only
> show semver tags. Try it and fall back to a real tag if needed.

### 9b. Update the dynamic config

Edit `dynamic.yml` to add the sni-check middleware:

```yaml
http:
  routers:
    legit:
      rule: "Host(`legit.example.com`)"
      entryPoints:
        - websecure
      service: legit
      tls: {}
      middlewares:
        - sni-check

    victim:
      rule: "Host(`victim.example.com`)"
      entryPoints:
        - websecure
      service: victim
      tls: {}
      middlewares:
        - sni-check

  middlewares:
    sni-check:
      plugin:
        traefik-sni: {}

  services:
    legit:
      loadBalancer:
        servers:
          - url: "http://legit:80"

    victim:
      loadBalancer:
        servers:
          - url: "http://victim:80"

tls:
  certificates:
    - certFile: /etc/traefik/certs/cert.pem
      keyFile: /etc/traefik/certs/key.pem
```

### 9c. Start Traefik with the plugin

```bash
docker run -d --name traefik --network sni-test \
  -p 443:443 \
  -v $(pwd)/traefik.yml:/etc/traefik/traefik.yml:ro \
  -v $(pwd)/dynamic.yml:/etc/traefik/dynamic/dynamic.yml:ro \
  -v $(pwd)/certs:/etc/traefik/certs:ro \
  traefik:v3.3
```

Check the logs to confirm the plugin was loaded:

```bash
docker logs traefik 2>&1 | grep -i plugin
```

You should see a log line about downloading/loading `traefik-sni`.

## Step 10: Phase 2 — prove the plugin blocks domain fronting

### 10a. Normal request (should still work)

```bash
curl -sk -o /dev/null -w '%{http_code}' https://legit.example.com/
```

Expected: `200`.

### 10b. Domain fronting attack (should now be blocked)

```bash
curl -sk -o /dev/null -w '%{http_code}' \
  --resolve legit.example.com:443:<DROPLET_IP> \
  -H "Host: victim.example.com" \
  https://legit.example.com/
```

Expected: `421` (Misdirected Request). The plugin detected that
SNI (`legit.example.com`) does not match Host (`victim.example.com`)
and rejected the request.

### 10c. Verify with verbose output

```bash
curl -sk -v \
  --resolve legit.example.com:443:<DROPLET_IP> \
  -H "Host: victim.example.com" \
  https://legit.example.com/ 2>&1 | grep "< HTTP"
```

Expected: `< HTTP/2 421`.

## Cleanup

```bash
docker rm -f traefik legit victim
docker network rm sni-test
```

If you created a tag for testing:

```bash
git tag -d v0.1.0
git push origin --delete v0.1.0
```

Destroy the droplet when done.

---

## Alternative: local plugin mode

If you don't want to make the repo public, clone it onto the droplet
and use Traefik's local plugin mechanism instead:

```bash
# On the droplet
cd ~/traefik-test
git clone git@github.com:kumojin/traefik-sni.git
```

Use `localPlugins` in the static config instead of `plugins`:

```yaml
experimental:
  localPlugins:
    traefik-sni:
      moduleName: github.com/kumojin/traefik-sni
```

And mount the repo into the Traefik container:

```bash
docker run -d --name traefik --network sni-test \
  -p 443:443 \
  -v $(pwd)/traefik.yml:/etc/traefik/traefik.yml:ro \
  -v $(pwd)/dynamic.yml:/etc/traefik/dynamic/dynamic.yml:ro \
  -v $(pwd)/certs:/etc/traefik/certs:ro \
  -v $(pwd)/traefik-sni:/plugins-local/src/github.com/kumojin/traefik-sni:ro \
  traefik:v3.3
```

This is identical to how the automated e2e tests work. The dynamic
config (`dynamic.yml`) stays exactly the same — the middleware reference
`traefik-sni` works with both local and remote plugins.
