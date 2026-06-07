#!/usr/bin/env bash
# Build admin SPA into internal/server/admin/static/dist for go:embed (adminui tag).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/admin"

if command -v pnpm >/dev/null 2>&1; then
  pnpm install --frozen-lockfile
  pnpm build
else
  corepack enable
  corepack prepare pnpm@9 --activate
  pnpm install --frozen-lockfile
  pnpm build
fi

test -f "$ROOT/internal/server/admin/static/dist/index.html"
