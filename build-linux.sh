#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "Go 1.26 or newer is required." >&2
  exit 1
fi
if ! command -v node >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1; then
  echo "Node.js 20.19 or newer and npm are required to build the Vue frontend." >&2
  exit 1
fi

NODE_VERSION="$(node -p 'process.versions.node')"
NODE_MAJOR="${NODE_VERSION%%.*}"
NODE_REST="${NODE_VERSION#*.}"
NODE_MINOR="${NODE_REST%%.*}"
if [ "$NODE_MAJOR" -lt 20 ] || { [ "$NODE_MAJOR" -eq 20 ] && [ "$NODE_MINOR" -lt 19 ]; }; then
  echo "Node.js 20.19 or newer is required; found $NODE_VERSION." >&2
  exit 1
fi

cd "$ROOT/frontend"
if [ -f package-lock.json ]; then
  npm ci --no-audit --no-fund
else
  npm install --no-audit --no-fund
fi
npm run build
cd "$ROOT"

test -f frontend/dist/index.html
rm -rf internal/httpapi/web
mkdir -p internal/httpapi/web
cp -a frontend/dist/. internal/httpapi/web/

go mod tidy
gofmt -w $(find cmd internal -type f -name '*.go' -print)
go test ./...
go vet ./...

mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/pingtai-linux-amd64 ./cmd/pingtai
cp .env.example dist/.env.example
cp README_GO.md dist/README.md
cp manage-go.sh dist/manage-go.sh
mkdir -p dist/deploy
cp deploy/* dist/deploy/
chmod +x dist/pingtai-linux-amd64 dist/manage-go.sh

if command -v zip >/dev/null 2>&1; then
  STAGING="$(mktemp -d)"
  trap 'rm -rf "$STAGING"' EXIT
  cp dist/pingtai-linux-amd64 dist/.env.example dist/README.md dist/manage-go.sh "$STAGING/"
  mkdir -p "$STAGING/deploy"
  cp deploy/* "$STAGING/deploy/"
  (cd "$STAGING" && zip -qr "$ROOT/dist/pingtai-linux-amd64-release.zip" .)
fi

echo "Ubuntu release build complete: $ROOT/dist/pingtai-linux-amd64"
