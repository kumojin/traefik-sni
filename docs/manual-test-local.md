# Testing with Local Plugin (Phase 2)

Prove the traefik-sni plugin blocks domain fronting using Traefik's local plugin mechanism.

> Complete [Phase 1](manual-test-vulnerability.md) first to confirm the vulnerability exists, then return here.

## Get the plugin source

On the server, clone the repo into the path Traefik expects for local plugins (relative to the working directory):

```bash
mkdir -p plugins-local/src/github.com/kumojin
git clone https://github.com/kumojin/traefik-sni.git \
  plugins-local/src/github.com/kumojin/traefik-sni
```

Or copy from your local machine:

```bash
scp -r ./traefik-sni root@<SERVER_IP>:~/traefik-test/plugins-local/src/github.com/kumojin/traefik-sni
```

## Static config

Create (or replace) `traefik.yml`:

```yaml
log:
  level: DEBUG

entryPoints:
  websecure:
    address: ":443"

experimental:
  localPlugins:
    traefik-sni:
      moduleName: github.com/kumojin/traefik-sni

providers:
  file:
    directory: ./dynamic
    watch: true
```

## Dynamic config

Create (or replace) `dynamic/dynamic.yml`:

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
          - url: "http://127.0.0.1:8001"

    victim:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:8002"

tls:
  certificates:
    - certFile: certs/cert.pem
      keyFile: certs/key.pem
```

## Start Traefik

```bash
traefik --configfile traefik.yml &
```

### Verify plugin loaded

```bash
traefik version
```

Check the log output for a line about loading `traefik-sni`.

## Tests

Run from your **local machine**, not the server.

### Normal request

```bash
curl -sk https://legit.example.com/
```

Expected: 200, body contains `Name: legit`.

### Domain fronting attack

```bash
curl -sk --resolve legit.example.com:443:<SERVER_IP> \
  -H "Host: victim.example.com" \
  https://legit.example.com/
```

Expected: **421 Misdirected Request**. The plugin detected the SNI/Host mismatch and rejected the request.

### Verbose check

```bash
curl -sk -v \
  --resolve legit.example.com:443:<SERVER_IP> \
  -H "Host: victim.example.com" \
  https://legit.example.com/ 2>&1 | grep "< HTTP"
```

Expected: `< HTTP/2 421`.

## Stop Traefik

```bash
pkill traefik
```

See [cleanup instructions](manual-test.md#cleanup) to tear down the full environment.
