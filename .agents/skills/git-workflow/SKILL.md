# Git Workflow

Conventions for commits, branches, pull requests, and releases in this repository.

## Commit Messages

Format: `<type>(<scope>): <description>`

Scope is optional. Description uses imperative mood, lowercase, no period, max 72 chars.

### Types

| Type | When to use |
|------|-------------|
| `feat` | New feature or behavior change |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `ci` | CI/CD workflow changes |
| `test` | Test changes only |
| `chore` | Maintenance (deps, config, tooling) |

### Examples

```
feat: add allowedHosts configuration parameter
feat(config): support audit-only mode
fix: normalize IPv6 addresses in Host header
docs: update README installation instructions
ci: pin golangci-lint-action to v9.2.0
test: add e2e test for empty SNI passthrough
chore: update testify to v1.12.0
```

## Branches

Format: `<type>/<short-description>`

Examples: `feat/allowed-hosts`, `fix/ipv6-normalization`, `docs/readme-update`

Create from `main`. Keep branches short-lived.

## Pull Requests

### Title

Same format as a commit message: `<type>(<scope>): <description>`

### Body

```markdown
## Summary

- Bullet point describing what changed and why
- Another bullet point if needed

## Testing

- How this was tested (e.g., "unit tests pass", "tested manually on droplet")
```

Include test evidence (command output, screenshots) when relevant.

### Merge

- Strategy: **squash merge** to `main`
- Delete the source branch after merge
- Ensure CI passes before merging

## Releases

- Tag format: `v<major>.<minor>.<patch>` (semver)
- Pushing a tag triggers the CD workflow, which creates a GitHub Release with auto-generated release notes
- Process:
  1. Merge PR to `main`
  2. Create tag: `git tag v1.0.0`
  3. Push tag: `git push origin v1.0.0`
  4. GitHub Release is auto-created by `cd.yaml`
