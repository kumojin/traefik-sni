# AGENTS.md

Reference for AI coding agents working on this repository.

## Project Overview

Traefik middleware plugin, interpreted by Yaegi at runtime (no compilation). Prevents domain fronting by comparing TLS SNI server name with HTTP Host header. Returns `421 Misdirected Request` on mismatch.

## Repository Structure

```
├── sni.go                         # Core middleware (Config, CreateConfig, New, ServeHTTP, normalizeHost)
├── sni_test.go                    # Unit tests (15 cases, testify assert/require/mock, map-driven table)
├── .traefik.yml                   # Traefik plugin manifest
├── go.mod / go.sum                # Go module (single dep: testify)
├── test/
│   ├── test.sh                    # Self-contained e2e test script (both phases, 4 tests)
│   ├── traefik.yml                # Traefik static config for tests
│   ├── traefik-dynamic-vulnerable.yml  # Dynamic config: no middleware
│   ├── traefik-dynamic-protected.yml   # Dynamic config: sni-check middleware
│   └── dynamic/                   # Runtime dir (gitignored, created by test.sh)
├── .github/
│   ├── actions/setup-traefik/     # Composite action for CI e2e tests
│   └── workflows/
│       ├── ci.yaml                # CI: unit-tests + lint + e2e-vulnerable + e2e-protected
│       └── cd.yaml                # CD: tag-triggered GitHub Release
├── docs/                          # Manual test plans
├── .agents/skills/                # AI agent skills (git workflow conventions)
├── README.md                      # Project documentation
├── AGENTS.md                      # This file
├── LICENSE                        # MIT
├── .gitignore                     # test/dynamic/
├── .golangci.yml                  # Linter config
└── justfile                       # Task runner (test, test-style, test-unit, test-e2e)
```

## Yaegi Constraints

- No `unsafe`, no cgo.
- Must export: `Config` struct, `CreateConfig() *Config`, `New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error)`.
- `.traefik.yml` manifest required with `displayName`, `type: middleware`, `import`, `summary`, `testData`.
- `log/slog` works fine with Traefik v3.3+ (Go 1.23 runtime) -- earlier suspicion about incompatibility was wrong.
- Config has three fields: `rejectOnMissingSNI`, `rejectOnMissingHost`, `logOnly` (all bool).

## Plugin Loading

Three methods exist:

1. **Plugin Catalog** (`experimental.plugins`) -- requires publication to plugins.traefik.io.
2. **Local Plugins** (`experimental.localPlugins`) -- mount source to `/plugins-local/src/...`. Used for dev/CI.
3. **Traefik Hub** private registry -- paid, not used here.

## Testing

```
just test-style   # golangci-lint run ./... (Docker fallback if not installed)
just test-unit    # go test -v -race ./...
just test-e2e     # ./test/test.sh (needs Docker)
just test         # all of the above
```

## Git & GitHub Conventions

See `.agents/skills/git-workflow/SKILL.md` if needed.

## Key Decisions

- `log/slog` is Yaegi-compatible with Traefik v3.3+ -- earlier hypothesis was wrong.
- Config struct has three fields: `rejectOnMissingSNI`, `rejectOnMissingHost`, `logOnly`.
- Wildcard TLS certs in tests model real shared-infrastructure domain fronting scenarios.
- `localPlugins` for dev/CI; Plugin Catalog for production distribution.
- Test tables use `map[string]struct{}` pattern (idiomatic Go, unordered).
- Raw `docker` commands preferred over `docker compose` in CI.
