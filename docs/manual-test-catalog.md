# Testing with the Plugin Catalog

This document will be completed when the plugin is published to the [Traefik Plugin Catalog](https://plugins.traefik.io).

## Prerequisites for publication

- Public GitHub repository
- `traefik-plugin` topic added to the repo
- `.traefik.yml` manifest in the repo root
- A git tag (semver, e.g., `v0.1.0`)
- Wait for the catalog to crawl (~30 minutes)

## Overview

Once published, the static config will use `experimental.plugins` with `moduleName` and `version` instead of `localPlugins`:

```yaml
experimental:
  plugins:
    traefik-sni-host-check:
      moduleName: github.com/DialogInsight/traefik-sni-host-check
      version: v0.1.0
```

The dynamic config (middleware definition and router references) remains the same as in the [local plugin test](manual-test-local.md). No local plugin source is needed -- Traefik downloads it from GitHub automatically.
