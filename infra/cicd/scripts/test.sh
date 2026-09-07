#!/usr/bin/env bash
# Runs build+vet+test for every Go module in the workspace. Mirrors exactly
# what .github/workflows/go.yml does per matrix leg, as a single local
# entrypoint — useful to run before pushing, or to call from a future CI
# step without duplicating the module list in two places.
#
# Assumes a MongoDB reachable at localhost:27017 for movies-service's
# integration tests (see docs/adr/0001, docs/adr/0002) — start one with:
#   docker run -d -p 27017:27017 mongo:7
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

for module in movies-service api-gateway proto shared; do
  echo "== ${module} =="
  (
    cd "${ROOT_DIR}/${module}"
    go build ./...
    go vet ./...
    go test -p 1 ./...
  )
done

echo "All modules passed."
