# Testing with Local Plugin (Phase 2)

Prove the traefik-sni plugin blocks domain fronting using Traefik's local plugin mechanism.

> Complete [Phase 1](manual-test-vulnerability.md) first to confirm the vulnerability exists, then return here.

## Get the plugin source

On the server, clone the repo into the working directory:

```bash
git clone https://github.com/kumojin/traefik-sni.git ~/traefik-test/traefik-sni
```

Or copy from your local machine:

```bash
scp -r ./traefik-sni root@<SERVER_IP>:~/traefik-test/
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
    directory: /etc/traefik/dynamic
    watch: true
```

## Dynamic config

Create (or replace) `dynamic.yml`:

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

## Start Traefik

```bash
docker run -d --name traefik --network sni-test \
  -p 443:443 \
  -v $(pwd)/traefik.yml:/etc/traefik/traefik.yml:ro \
  -v $(pwd)/dynamic.yml:/etc/traefik/dynamic/dynamic.yml:ro \
  -v $(pwd)/certs:/etc/traefik/certs:ro \
  -v $(pwd)/traefik-sni:/plugins-local/src/github.com/kumojin/traefik-sni:ro \
  traefik:v3.3
```

> The extra volume mount maps the plugin source into the path Traefik expects for local plugins.

### Verify plugin loaded

```bash
docker logs traefik 2>&1 | grep -i plugin
```

You should see a log line about loading `traefik-sni`.

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
docker rm -f traefik
```

See [cleanup instructions](manual-testing.md#cleanup) to tear down the full environment.
