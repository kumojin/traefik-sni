# Testing with the Plugin (Phase 2)

Prove the traefik-sni-host-check plugin blocks domain fronting.

> Complete [Phase 1](manual-test-vulnerable.md) first to confirm the vulnerability exists, then return here.

Pick one of the two options below to load the plugin, then continue with the shared dynamic config and tests.

## Option A: Local plugin

Use this during development or before the plugin is published to the catalog.

### Get the plugin source

On the server, clone the repo into the path Traefik expects for local plugins (relative to the working directory):

```bash
mkdir -p plugins-local/src/github.com/DialogInsight
git clone https://github.com/DialogInsight/traefik-sni-host-check.git \
  plugins-local/src/github.com/DialogInsight/traefik-sni-host-check
```

Or copy from your local machine:

```bash
scp -r ./traefik-sni-host-check root@<SERVER_IP>:~/traefik-test/plugins-local/src/github.com/DialogInsight/traefik-sni-host-check
```

### Static config

```bash
cat <<'EOF' > traefik-static-prot.yml
log:
  level: DEBUG

entryPoints:
  websecure:
    address: ":443"

experimental:
  localPlugins:
    traefik-sni-host-check:
      moduleName: github.com/DialogInsight/traefik-sni-host-check

providers:
  file:
    filename: ./traefik-dynamic-prot.yml
    watch: true
EOF
```

## Option B: Plugin Catalog

Use this once the plugin is published to the [Traefik Plugin Catalog](https://plugins.traefik.io).

### Prerequisites for publication

- Public GitHub repository
- `traefik-plugin` topic added to the repo
- `.traefik.yml` manifest in the repo root
- A git tag (semver, e.g., `v0.1.0`)
- Wait for the catalog to crawl (~30 minutes)

### Static config

No local plugin source is needed -- Traefik downloads it from GitHub automatically.

```bash
cat <<'EOF' > traefik-static-prot.yml
log:
  level: DEBUG

entryPoints:
  websecure:
    address: ":443"

experimental:
  plugins:
    traefik-sni-host-check:
      moduleName: github.com/DialogInsight/traefik-sni-host-check
      version: v0.1.0

providers:
  file:
    filename: ./traefik-dynamic-prot.yml
    watch: true
EOF
```

## Dynamic config

The dynamic config is the same regardless of how the plugin is loaded.

```bash
cat <<'EOF' > traefik-dynamic-prot.yml
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
        traefik-sni-host-check:
          rejectOnMissingSNI: true
          rejectOnMissingHost: false
          logOnly: false
          logLevel: INFO
          logFilePath: ""
          logFormat: common

  services:
    legit:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:8001"

    victim:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:8002"

tls:
  certificates:
    - certFile: certs/cert.pem
      keyFile: certs/key.pem
EOF
```

## Start Traefik

```bash
traefik --configfile traefik-static-prot.yml
```

Check the log output for startup lines from the plugin. There should be two pairs (one per router), each with an INFO and a DEBUG line similar to the following:

```plain
time=2026-04-08T16:39:43.879Z level=INFO msg="plugin started" middleware=sni-check@file
time=2026-04-08T16:39:43.879Z level=DEBUG msg=configuration middleware=sni-check@file rejectOnMissingSNI=true rejectOnMissingHost=false logOnly=false logLevel=INFO logFilePath="" logFormat=common
```

## Tests

Run from your **local machine**, not the server.

### Normal request

```bash
curl -sk https://legit.example.com/
```

Expected: 200, body contains `Name: legit`. As before, the request should trigger a log line similar to the following:

```plain
2026-04-08T16:41:23Z DBG github.com/traefik/traefik/v3/pkg/server/service/loadbalancer/wrr/wrr.go:213 > Service selected by WRR: http://127.0.0.1:8001
```

### Domain fronting attack

```bash
curl -sk --resolve legit.example.com:443:<SERVER_IP> \
  -H "Host: victim.example.com" \
  https://legit.example.com/
```

Expected: **421 Misdirected Request**. The plugin detected the SNI/Host mismatch and rejected the request. A warning log should indicate this:

```plain
time=2026-04-08T16:42:15.356Z level=WARN msg="misdirected request" middleware=sni-check@file sni=legit.example.com host=victim.example.com
```

### Verbose check

```bash
curl -sk -v \
  --resolve legit.example.com:443:<SERVER_IP> \
  -H "Host: victim.example.com" \
  https://legit.example.com/ 2>&1 | grep "< HTTP"
```

Expected: `< HTTP/2 421`.

### Missing SNI (rejectOnMissingSNI)

Connect directly to the server IP instead of using the domain name. Per RFC 6066, TLS clients must not send SNI for IP addresses, so Traefik receives an empty server name.

```bash
curl -sk -H "Host: legit.example.com" https://<SERVER_IP>/
```

Expected with `rejectOnMissingSNI: true` (default): **421 Misdirected Request**.

To verify the plugin allows it when configured, set `rejectOnMissingSNI: false` in `traefik-dynamic-prot.yml`, wait for Traefik to reload, and repeat the command. Expected: 200, body contains `Name: legit`.

> **Note:** `rejectOnMissingHost` cannot be tested via curl. Traefik's router needs the Host header to match `Host(...)` rules and route the request to the middleware. Without it, Traefik returns 404 before the middleware runs. The setting exists as defense-in-depth for edge cases (e.g., health check probes, other middleware stripping the Host header upstream).

See [cleanup instructions](manual-test.md#cleanup) to tear down the full environment.
