#!/usr/bin/env bash
# Builds both service images. Run from the repo root or anywhere — paths
# below are resolved relative to this script, not the caller's cwd.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT_DIR"

echo "Building movies-service..."
docker build -f movies-service/Dockerfile -t movies-service:latest .

echo "Building api-gateway..."
docker build -f api-gateway/Dockerfile -t api-gateway:latest .

echo "Done: movies-service:latest, api-gateway:latest"
