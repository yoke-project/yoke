#!/usr/bin/env bash
# The conformance suite, L2: this checkout's suite and Core against the Go family's plugin harness.
#
# The harness comes from the module proxy at the version pinned here, never from a sibling; moving it is
# a change like any other. The suite's lines go to standard output and to the results directory, where
# the record writer reads them. Usage: conformance.sh [results directory]
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
results="${1:-$root/.results}"
harness=github.com/yoke-project/yoke-sdk-go/cmd/yoke-go-plugin-harness@v0.0.0-20260926132730-9b02de5ffa98
bin="$(mktemp -d)"
trap 'rm -rf "$bin"' EXIT
mkdir -p "$results"

GOBIN="$bin" go install "$harness" || exit 1
go -C "$root" build -o "$bin/yoke-core" ./cmd/yoke-core || exit 1
go -C "$root" build -o "$bin/yoke-conformance" ./cmd/yoke-conformance || exit 1

"$bin/yoke-conformance" --core "$bin/yoke-core" --harness "$bin/yoke-go-plugin-harness" | tee "$results/conformance.txt"
exit "${PIPESTATUS[0]}"
