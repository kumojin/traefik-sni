#!/usr/bin/env bash
#
# End-to-end tests for the traefik-sni plugin.
#
# Self-contained: generates certs, starts containers with raw docker commands,
# runs tests, and cleans up. No docker compose required.
#
# Demonstrates domain fronting in two phases:
#   Phase 1 (vulnerable) — no middleware, attack succeeds
#   Phase 2 (protected)  — sni-check middleware blocks the attack
#
# Requires: docker, curl, openssl.
#
# Usage:
#   ./test/test.sh
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIGS_DIR="$SCRIPT_DIR/configs"
DYNAMIC_DIR="$SCRIPT_DIR/dynamic"
CERTS_DIR="$(mktemp -d)"

# Container/network names, prefixed to avoid collisions.
NET="sni-test-net"
CTR_LEGIT="sni-test-legit"
CTR_VICTIM="sni-test-victim"
CTR_TRAEFIK="sni-test-traefik"

PASS=0
FAIL=0

# -------------------------------------------------------------------
# Cleanup: always tear down containers and network on exit.
# -------------------------------------------------------------------
cleanup() {
  echo ""
  echo "Cleaning up..."
  docker rm -f "$CTR_TRAEFIK" "$CTR_LEGIT" "$CTR_VICTIM" 2>/dev/null || true
  docker network rm "$NET" 2>/dev/null || true
  rm -f "$DYNAMIC_DIR/active.yml"
  rm -rf "$CERTS_DIR"
  echo "Done."
}
trap cleanup EXIT

# -------------------------------------------------------------------
# Generate self-signed certs in a temporary directory.
# -------------------------------------------------------------------
generate_certs() {
  echo "Generating self-signed TLS certificates..."
  openssl req -x509 -newkey rsa:2048 \
    -keyout "$CERTS_DIR/key.pem" \
    -out "$CERTS_DIR/cert.pem" \
    -days 365 -nodes \
    -subj "/CN=legit.localhost" \
    -addext "subjectAltName=DNS:legit.localhost" \
    2>/dev/null
  echo "Certs generated."
}

# -------------------------------------------------------------------
# Test helpers.
# -------------------------------------------------------------------
check() {
  local description="$1"
  local expected_status="$2"
  shift 2
  local actual_status
  actual_status=$(curl --silent --max-time 5 --output /dev/null --write-out "%{http_code}" "$@") || true

  if [ "$actual_status" = "$expected_status" ]; then
    echo "  PASS  $description (got $actual_status)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $description (expected $expected_status, got $actual_status)"
    FAIL=$((FAIL + 1))
  fi
}

check_body() {
  local description="$1"
  local expected_status="$2"
  local expected_body="$3"
  shift 3

  local tmpfile
  tmpfile=$(mktemp)
  local actual_status
  actual_status=$(curl --silent --max-time 5 --output "$tmpfile" --write-out "%{http_code}" "$@") || true
  local body
  body=$(cat "$tmpfile")
  rm -f "$tmpfile"

  if [ "$actual_status" != "$expected_status" ]; then
    echo "  FAIL  $description (expected status $expected_status, got $actual_status)"
    FAIL=$((FAIL + 1))
    rm -f "$tmpfile"
    return
  fi

  if echo "$body" | grep -q "$expected_body"; then
    echo "  PASS  $description (status $actual_status, body contains \"$expected_body\")"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $description (status OK but body missing \"$expected_body\")"
    echo "        body: $(echo "$body" | head -5)"
    FAIL=$((FAIL + 1))
  fi
}

# wait_for_status polls a curl command until the expected HTTP status is
# returned, or times out after ~30 seconds.
wait_for_status() {
  local description="$1"
  local expected_status="$2"
  shift 2
  local attempts=0
  local max_attempts=30

  echo -n "  Waiting: $description "
  while [ $attempts -lt $max_attempts ]; do
    local status
    status=$(curl --silent --max-time 2 --output /dev/null --write-out "%{http_code}" "$@" 2>/dev/null) || true
    if [ "$status" = "$expected_status" ]; then
      echo " ready."
      return 0
    fi
    echo -n "."
    attempts=$((attempts + 1))
    sleep 1
  done
  echo " TIMEOUT"
  return 1
}

activate_config() {
  local config="$1"
  mkdir -p "$DYNAMIC_DIR"
  rm -f "$DYNAMIC_DIR/active.yml"
  cp "$CONFIGS_DIR/${config}.yml" "$DYNAMIC_DIR/active.yml"
}

# ===================================================================
# Setup
# ===================================================================

echo ""
echo "=========================================="
echo " Setup"
echo "=========================================="
echo ""

generate_certs

# Create an isolated Docker network.
docker network create "$NET" >/dev/null 2>&1

# Start the two whoami backends.
# --network-alias gives them DNS names matching the service URLs in the
# dynamic config (http://legit:80, http://victim:80).
docker run -d --name "$CTR_LEGIT"  --network "$NET" --network-alias legit  traefik/whoami --name legit >/dev/null
docker run -d --name "$CTR_VICTIM" --network "$NET" --network-alias victim traefik/whoami --name victim >/dev/null

# Place the initial (vulnerable) config before Traefik starts.
activate_config vulnerable

# Start Traefik with the plugin mounted locally.
docker run -d --name "$CTR_TRAEFIK" --network "$NET" \
  -p 443:443 \
  -v "$SCRIPT_DIR/traefik.yml:/etc/traefik/traefik.yml:ro" \
  -v "$DYNAMIC_DIR:/etc/traefik/dynamic" \
  -v "$CERTS_DIR:/etc/traefik/certs:ro" \
  -v "$PROJECT_ROOT:/plugins-local/src/github.com/kumojin/traefik-sni:ro" \
  traefik:v3.3 >/dev/null

# Wait for Traefik to be ready (serves a normal request).
wait_for_status "Traefik ready" "200" \
  --insecure \
  --resolve legit.localhost:443:127.0.0.1 \
  https://legit.localhost/

# ===================================================================
# Phase 1: VULNERABLE (no middleware)
# ===================================================================

echo ""
echo "=========================================="
echo " Phase 1: VULNERABLE (no middleware)"
echo "=========================================="
echo ""
echo " Proving that domain fronting works when"
echo " Traefik has no SNI/Host check."
echo ""

# 1a. Normal request to legit works.
check_body "legit request to legit backend" \
  "200" "Name: legit" \
  --insecure \
  --resolve legit.localhost:443:127.0.0.1 \
  https://legit.localhost/

# 1b. Domain fronting: TLS connects to legit (SNI=legit.localhost),
#     but the Host header says victim.localhost.
#     Traefik routes on Host, so the request reaches the VICTIM backend.
check_body "DOMAIN FRONTING: SNI=legit, Host=victim -> reaches victim" \
  "200" "Name: victim" \
  --insecure \
  --resolve legit.localhost:443:127.0.0.1 \
  -H "Host: victim.localhost" \
  https://legit.localhost/

# ===================================================================
# Phase 2: PROTECTED (sni-check middleware)
# ===================================================================

echo ""
echo "=========================================="
echo " Phase 2: PROTECTED (sni-check middleware)"
echo "=========================================="
echo ""
echo " Same attack, now blocked by the plugin."
echo ""

# Swap to the protected config and wait for Traefik to reload.
activate_config protected

wait_for_status "config reload (domain fronting now returns 421)" "421" \
  --insecure \
  --resolve legit.localhost:443:127.0.0.1 \
  -H "Host: victim.localhost" \
  https://legit.localhost/

# 2a. Normal request to legit still works.
check_body "legit request to legit backend (protected)" \
  "200" "Name: legit" \
  --insecure \
  --resolve legit.localhost:443:127.0.0.1 \
  https://legit.localhost/

# 2b. Domain fronting: same attack as before, now returns 421.
check "DOMAIN FRONTING BLOCKED: SNI=legit, Host=victim -> 421" \
  "421" \
  --insecure \
  --resolve legit.localhost:443:127.0.0.1 \
  -H "Host: victim.localhost" \
  https://legit.localhost/

# ===================================================================
# Results
# ===================================================================

echo ""
echo "=========================================="
echo " Results: $PASS passed, $FAIL failed"
echo "=========================================="
echo ""

if [ "$FAIL" -gt 0 ]; then
  echo "Traefik logs:"
  echo "---"
  docker logs "$CTR_TRAEFIK" 2>&1
  echo "---"
  exit 1
fi
