#!/usr/bin/env bash
# Assembles the record of a run from what that run left behind, and writes it to standard output.
# It runs nothing: a record is evidence of a run that already happened.
# Usage: record.sh [results directory]
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
results="${1:-$root/.results}"

for each in checks.txt started finished; do
  [[ -f "$results/$each" ]] || { echo "record: the run left no $each in $results" >&2; exit 1; }
done

# The architecture as the environment's dimension names it.
case "$(uname -m)" in
  x86_64) architecture=amd64 ;;
  aarch64 | arm64) architecture=arm64 ;;
  *) architecture="$(uname -m)" ;;
esac

arguments=(record
  --level L1
  --tier reference
  --repository yoke
  --environment "architecture=$architecture"
  --started "$(tr -d '[:space:]' < "$results/started")"
  --finished "$(tr -d '[:space:]' < "$results/finished")"
  --ran "yoke-verify=unreleased@$(git -C "$root" rev-parse --short HEAD 2>/dev/null)"
  --results "$results/checks.txt")

[[ -s "$results/go.json" ]] && arguments+=(--results "$results/go.json")

go run ./cmd/yoke-verify "${arguments[@]}" "$root"
