#!/usr/bin/env bash
# The definitions' tools, each run from source through the module proxy, so that changing the definitions
# needs Go and `just` and nothing else. The plugins'
# versions are in proto/buf.gen.yaml; buf's is here.
#
# Usage: definitions.sh check              build, lint, format and the two version statements (A6)
#        definitions.sh generate <dir>     the Go the definitions generate, into <dir>, relative to proto/
set -euo pipefail

buf=(go run github.com/bufbuild/buf/cmd/buf@v1.73.0)
cd "$(dirname "${BASH_SOURCE[0]}")/../proto"

case "${1:-}" in
  check)
    "${buf[@]}" build
    "${buf[@]}" lint
    "${buf[@]}" format --diff --exit-code
    go run ./internal/contracts .
    echo "definitions: they build, lint clean, are formatted, and every contract states its path's integer"
    ;;
  generate)
    "${buf[@]}" generate --output "${2:?usage: definitions.sh generate <dir>}"
    ;;
  *)
    echo "usage: definitions.sh check | generate <dir>" >&2
    exit 2
    ;;
esac
