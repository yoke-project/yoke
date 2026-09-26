#!/usr/bin/env bash
# Assembles the record of one level of a run from what that run left behind, and writes it to standard
# output. It runs nothing: a record is evidence of a run that already happened. One run of `test`
# performs L1, L2 and L3, and each level's record takes the cases declared at it.
#
# A level whose predecessor blocked was not reached: PREDECESSOR names that record — L1 for L2, L2 for
# L3.
# Usage: [PREDECESSOR=<record>] record.sh <level> [results directory]
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
level="${1:?usage: record.sh <level> [results directory]}"
results="${2:-$root/.results}"

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
  --level "$level"
  --tier reference
  --repository yoke
  --environment "architecture=$architecture"
  --started "$(tr -d '[:space:]' < "$results/started")"
  --finished "$(tr -d '[:space:]' < "$results/finished")"
  --ran "yoke-verify=unreleased@$(git -C "$root" rev-parse --short HEAD 2>/dev/null)"
  --results "$results/checks.txt")

[[ -s "$results/go.json" ]] && arguments+=(--results "$results/go.json")
[[ -s "$results/conformance.txt" ]] && arguments+=(--results "$results/conformance.txt")

if [[ -n "${PREDECESSOR:-}" ]]; then
  blocked="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("blocks") is not False)' "$PREDECESSOR" 2>/dev/null)"
  # A predecessor whose record cannot be read did not pass either.
  [[ "$blocked" == False ]] || arguments+=(--not-reached)
fi

go -C "$root" run ./cmd/yoke-verify "${arguments[@]}" "$root"
