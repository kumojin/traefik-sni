set shell := ["bash", "-o", "pipefail", "-cu"]

golangci_version := "2.11.4"

default:
  @just --list

# Run all checks: style + unit tests + e2e tests.
test: test-style test-unit test-e2e

# Run golangci-lint (falls back to Docker if not installed locally).
test-style:
  #!/usr/bin/env sh
  if command -v golangci-lint > /dev/null 2>&1; then
    just _test-style-local
  else
    just _test-style-docker
  fi

_test-style-local:
  #!/usr/bin/env sh
  installed=$(golangci-lint version --short)
  if [ "$installed" != "{{golangci_version}}" ]; then
    echo "Error: golangci-lint version: expected \`{{golangci_version}}\`, actual \`${installed}\`" >&2
    exit 1
  fi
  golangci-lint run ./...

_test-style-docker:
  docker run --rm --workdir /app --volume ./:/app golangci/golangci-lint:v{{golangci_version}}-alpine golangci-lint run ./...

# Run unit tests with race detection.
test-unit:
  go test -v -race ./...

# Run end-to-end tests (requires Docker).
test-e2e:
  ./test/test.sh
