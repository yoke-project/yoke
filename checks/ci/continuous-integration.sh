#!/usr/bin/env bash
# The checks described by continuous-integration.std.md, one function per case.

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
workflow="$root/.github/workflows/verify.yml"

# The command of every `run:` step, one per line; a multi-line `run: |` block is refused outright.
run_commands() {
  grep -E '^[[:space:]]*(- )?run:' "$workflow" | sed -E 's/^[[:space:]]*(- )?run:[[:space:]]*//'
}

# std: yoke:continuous-integration.01
check_verbs_on_every_change() {
  [[ -f "$workflow" ]] || { echo "no workflow"; return 1; }
  grep -qE '^[[:space:]]*pull_request:' "$workflow" || { echo "the workflow does not run on a proposed change"; return 1; }
  local commands verb
  commands="$(run_commands)"
  for verb in build test lint fmt; do
    grep -qx "just $verb" <<<"$commands" || { echo "'just $verb' is not run"; return 1; }
  done
  local other
  other="$(grep -vE '^just (build|test|lint|fmt)$' <<<"$commands" | grep -vE '^ci/' || true)"
  [[ -z "$other" ]] || { echo "a step does work of its own: $other"; return 1; }
}

# std: yoke:continuous-integration.02
check_document_selects_nothing() {
  [[ -x "$root/ci/select-jobs.sh" ]] || { echo "no path filter"; return 1; }
  local selected
  selected="$(printf 'README.md\n' | "$root/ci/select-jobs.sh")"
  [[ -z "$selected" ]] || { echo "a document selected: $selected"; return 1; }
}

# std: yoke:continuous-integration.03
check_definitions_select_suite_and_comparison() {
  [[ -x "$root/ci/select-jobs.sh" ]] || { echo "no path filter"; return 1; }
  local selected job
  selected="$(printf 'definitions/yoke/plugin/v1/plugin.proto\n' | "$root/ci/select-jobs.sh")"
  for job in verify suite version-comparison; do
    grep -qx "$job" <<<"$selected" || { echo "a definitions change did not select '$job'"; return 1; }
  done
}

# std: yoke:continuous-integration.04
check_toolchain_declared() {
  [[ -f "$workflow" ]] || { echo "no workflow"; return 1; }
  local runners
  runners="$(grep -E '^[[:space:]]*runs-on:' "$workflow" | sed -E 's/^[[:space:]]*runs-on:[[:space:]]*//')"
  [[ -n "$runners" ]] || { echo "no job names its runner"; return 1; }
  if grep -qvE '^ubuntu-[0-9]+\.[0-9]+$' <<<"$runners"; then echo "a runner image is not pinned: $runners"; return 1; fi
  grep -qE 'just-version:[[:space:]]*[0-9]' "$workflow" || { echo "the runner's version is not stated"; return 1; }
  grep -qE 'go-version:[[:space:]]*.?[0-9]' "$workflow" || { echo "the Go version is not stated"; return 1; }
}
