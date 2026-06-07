#!/usr/bin/env bash
# Start inlets server with demo config + seeded admin history for UI preview.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CONFIG="${INLETS_DEMO_CONFIG:-conf/example/server.demo.yaml}"
ADMIN_URL="${INLETS_DEMO_ADMIN_URL:-http://127.0.0.1:19090/}"

echo "==> Building admin UI..."
bash scripts/ci-build-admin.sh

echo "==> Seeding demo SQLite (revisions, audit, metrics)..."
go run ./scripts/seed-admin-demo -config "$CONFIG"

echo ""
echo "Demo ready. Starting server with: $CONFIG"
echo "  Admin console: $ADMIN_URL"
echo "  Tunnel ports:  HTTP 18080, TCP 18443 (avoid conflicts with defaults)"
echo "  Frontend dev:  INLETS_ADMIN_PROXY=http://127.0.0.1:19090 cd admin && pnpm dev"
echo ""

exec go run -tags adminui ./cmd/inlets server -c "$CONFIG" -p 18080 --tcp-port 18443
