#!/usr/bin/env bash
# The conformance suite, L2: this checkout's suite and Core against the Go family's harnesses, the plugin
# one and the administrative one.
#
# The harnesses come from the module proxy at the version pinned here, never from a sibling; moving it is
# a change like any other. The suite's lines go to standard output and to the results directory, where
# the record writer reads them. Usage: conformance.sh [results directory]
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
results="${1:-$root/.results}"
family=v0.2.1-0.20261004072756-5cb01418d590
bin="$(mktemp -d)"
trap 'rm -rf "$bin"' EXIT
mkdir -p "$results"

GOBIN="$bin" go install "github.com/yoke-project/yoke-sdk-go/cmd/yoke-go-plugin-harness@$family" || exit 1
GOBIN="$bin" go install "github.com/yoke-project/yoke-sdk-go/cmd/yoke-go-admin-harness@$family" || exit 1
go -C "$root" build -o "$bin/yoke-core" ./cmd/yoke-core || exit 1
go -C "$root" build -o "$bin/yoke-conformance" ./cmd/yoke-conformance || exit 1

status=0
"$bin/yoke-conformance" --core "$bin/yoke-core" --harness "$bin/yoke-go-plugin-harness" | tee "$results/conformance.txt"
(( PIPESTATUS[0] == 0 )) || status=1
"$bin/yoke-conformance" --core "$bin/yoke-core" --harness "$bin/yoke-go-admin-harness" | tee -a "$results/conformance.txt"
(( PIPESTATUS[0] == 0 )) || status=1
exit "$status"
