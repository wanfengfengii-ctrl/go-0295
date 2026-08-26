#!/usr/bin/env bash
# Smoke test for the rock wool facade handover service. It builds the binary,
# starts the service on a local loopback address, probes its real HTTP health
# and task endpoints, then cleans up every process and temporary file. It makes
# no external network requests and does not merely call `go test`.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

PORT="${SMOKE_PORT:-18080}"
TMPDIR="$(mktemp -d)"
PID=""

cleanup() {
  if [[ -n "${PID}" ]] && kill -0 "${PID}" 2>/dev/null; then
    kill "${PID}" 2>/dev/null || true
    wait "${PID}" 2>/dev/null || true
  fi
  rm -rf "${TMPDIR}"
}
trap cleanup EXIT

echo "building server binary..."
go build -o "${TMPDIR}/server" ./cmd/server

echo "starting service on 127.0.0.1:${PORT}..."
ADDR="127.0.0.1:${PORT}" DB_PATH="${TMPDIR}/smoke.db" "${TMPDIR}/server" &
PID=$!

# Wait for the health endpoint to come up, capturing the response in a file.
ready=0
for _ in $(seq 1 50); do
  if curl -fsS "http://127.0.0.1:${PORT}/healthz" > "${TMPDIR}/health.json" 2>/dev/null; then
    ready=1
    break
  fi
  sleep 0.1
done
if [[ "${ready}" -ne 1 ]]; then
  echo "service did not become ready" >&2
  exit 1
fi

HEALTH="$(cat "${TMPDIR}/health.json")"
if ! grep -q '"status":"ok"' <<<"${HEALTH}"; then
  echo "unexpected health response: ${HEALTH}" >&2
  exit 1
fi

echo "creating a facade task..."
CREATE="$(curl -fsS -X POST "http://127.0.0.1:${PORT}/v1/tasks" \
  -H 'Content-Type: application/json' \
  -d '{"building":"b1","facade_zone":"z1","wall_type":"concrete"}')"
if ! grep -q '"b1/z1"' <<<"${CREATE}"; then
  echo "unexpected create response: ${CREATE}" >&2
  exit 1
fi

echo "querying the task..."
GET="$(curl -fsS "http://127.0.0.1:${PORT}/v1/tasks/b1%2Fz1")"
if ! grep -q '"status":"created"' <<<"${GET}"; then
  echo "unexpected get response: ${GET}" >&2
  exit 1
fi

echo "smoke ok"
