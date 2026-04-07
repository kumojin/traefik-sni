# traefik-sni

A Traefik middleware plugin that prevents domain fronting by comparing the TLS SNI server name with the HTTP Host header.

## What is domain fronting?

Domain fronting exploits the gap between TLS-level and HTTP-level routing. The TLS Client Hello sends one hostname (SNI), but the encrypted HTTP request uses a different one (Host header). Reverse proxies like Traefik route on the Host header, so the attacker's request reaches a backend the TLS handshake wasn't intended for. This bypasses network-level access controls that inspect only the SNI.

```
Client → TLS SNI: legit.example.com  → Traefik → Routes on Host header
         HTTP Host: victim.example.com         → victim backend (!)
```

## What this plugin does

The plugin compares the TLS SNI value with the HTTP Host header on every request. When they don't match, it returns **HTTP 421 Misdirected Request** and logs a warning. Both values are normalized before comparison: ports are stripped, trailing FQDN dots are removed, and the comparison is case-insensitive.

| Condition | Result |
|-----------|--------|
| No TLS (`req.TLS == nil`) | Pass through |
| Empty SNI | **421** if `rejectOnMissingSNI` (default), else pass through |
| Empty Host | **421** if `rejectOnMissingHost`, else pass through |
| SNI matches Host | Pass through |
| SNI does not match Host | **421 Misdirected Request** |

In `logOnly` mode, all violations are logged but requests are allowed through.

## Installation

### Local plugin

For development and testing, mount the plugin source into the Traefik container:

```sh
docker run -v /path/to/traefik-sni:/plugins-local/src/github.com/kumojin/traefik-sni traefik
```

Static configuration (`traefik.yml`):

```yaml
experimental:
  localPlugins:
    traefik-sni:
      moduleName: github.com/kumojin/traefik-sni
```

### Plugin Catalog

For production, the plugin published in the [Traefik Plugin Catalog](https://plugins.traefik.io) is used:

```yaml
experimental:
  plugins:
    traefik-sni:
      moduleName: github.com/kumojin/traefik-sni
      version: v0.1.0
```

## Configuration

Dynamic configuration (`dynamic.yml`):

```yaml
http:
  middlewares:
    sni-check:
      plugin:
        traefik-sni:
          rejectOnMissingSNI: true
          rejectOnMissingHost: false
          logOnly: false

  routers:
    my-router:
      rule: "Host(`example.com`)"
      middlewares:
        - sni-check
      tls: {}
      service: my-service
```

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `rejectOnMissingSNI` | bool | `true` | Reject TLS requests where the client omitted the SNI extension. |
| `rejectOnMissingHost` | bool | `false` | Reject TLS requests with an empty HTTP Host header. |
| `logOnly` | bool | `false` | Log violations without blocking requests. Useful for safe rollout before enforcing. |

Use `logOnly: true` for safe rollout: deploy the middleware, observe logs to confirm no legitimate traffic is flagged, then switch to `logOnly: false`.

`rejectOnMissingSNI` defaults to `true` because a client can bypass the middleware entirely by omitting the SNI extension from the TLS ClientHello. Operators with legitimate empty-SNI traffic (rare) can set this to `false`.

`rejectOnMissingHost` defaults to `false` because an empty Host header is nearly impossible in practice: HTTP/1.1 mandates the Host header, and HTTP/2+ always provides `:authority`.

## Development

### Prerequisites

- Go 1.22+
- Docker
- [just](https://github.com/casey/just) (task runner)

### Commands

```sh
just test-style   # Run linter
just test-unit    # Run unit tests
just test-e2e     # Run end-to-end tests
just test         # Run all of the above
```

The end-to-end test is a self-contained script at [`./test/test.sh`](./test/test.sh).

## Manual testing

See the [manual testing guide](docs/manual-test.md) for instructions on testing the plugin on a real server.
