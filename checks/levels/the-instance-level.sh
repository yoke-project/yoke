#!/usr/bin/env bash
# The checks described by the-instance-level.std.md, one function per case.
# The Core is built into a directory of the check's own, and runs only against directories it creates.

il_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
il_workflow="$il_root/.github/workflows/verify.yml"

# std: yoke:the-instance-level.01
check_a_run_hands_over_a_record_per_level() {
  python3 - "$il_workflow" <<'PY'
import re, sys
text = open(sys.argv[1]).read()
steps = re.split(r"\n\s*- ", text)
for level in ("L1", "L3"):
    writes = [s for s in steps if re.search(rf"run: ci/record\.sh {level} > record-{level}\.json", s)]
    if not writes:
        raise SystemExit(f"no step writes the record of {level}")
    if "if: always()" not in writes[0]:
        raise SystemExit(f"the record of {level} is not written whatever the verbs decided")
    uploads = [s for s in steps if f"name: record-{level}" in s and f"path: record-{level}.json" in s]
    if not uploads:
        raise SystemExit(f"the record of {level} is not uploaded as record-{level}")
PY
}

# A results directory holding one passing check line, for the record script to read.
il_results() {
  mkdir -p "$1"
  printf 'pass  yoke:the-instance-level.03\n' > "$1/checks.txt"
  date -u +%Y-%m-%dT%H:%M:%SZ > "$1/started"
  date -u +%Y-%m-%dT%H:%M:%SZ > "$1/finished"
}

# std: yoke:the-instance-level.02
check_the_instance_level_waits_for_the_unit_level() {
  local tmp verdict=0 state; tmp="$(mktemp -d)"
  il_results "$tmp/results"
  printf '{"schema": 1, "level": "L1", "state": "failed", "blocks": true}\n' > "$tmp/blocking.json"
  printf '{"schema": 1, "level": "L1", "state": "passed", "blocks": false}\n' > "$tmp/passing.json"
  state="$(PREDECESSOR="$tmp/blocking.json" "$il_root/ci/record.sh" L3 "$tmp/results" 2>"$tmp/said" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["state"])' 2>/dev/null)"
  [[ "$state" == "not reached" ]] || { echo "after a blocking L1, L3 is '$state': $(head -c 300 "$tmp/said")"; verdict=1; }
  state="$(PREDECESSOR="$tmp/passing.json" "$il_root/ci/record.sh" L3 "$tmp/results" 2>"$tmp/said" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["state"])' 2>/dev/null)"
  [[ -n "$state" && "$state" != "not reached" ]] || { echo "after a passing L1, L3 is '$state': $(head -c 300 "$tmp/said")"; verdict=1; }
  rm -rf "$tmp"
  return "$verdict"
}

# std: yoke:the-instance-level.03
check_a_core_killed_leaves_its_instance_to_the_next() {
  local tmp verdict=0 first second third; tmp="$(mktemp -d)"
  (cd "$il_root" && go build -o "$tmp/yoke-core" ./cmd/yoke-core) || { echo "yoke-core does not build"; rm -rf "$tmp"; return 1; }
  printf 'state_dir: %s/state\nruntime_dir: %s/run\n' "$tmp" "$tmp" > "$tmp/core.yaml"
  export_env=(env YOKE_CONFIG="$tmp/core.yaml" YOKE_COMPOSITION="$tmp/deployment.yaml")

  "${export_env[@]}" "$tmp/yoke-core" 2>"$tmp/first.err" & first=$!
  for _ in $(seq 100); do [[ -e "$tmp/run/instance.lock" ]] && break; sleep 0.05; done
  sleep 0.2
  if "${export_env[@]}" "$tmp/yoke-core" 2>"$tmp/second.err"; then
    echo "a second Core started on a served instance"; verdict=1
  elif ! grep -q "already running, as process $first" "$tmp/second.err"; then
    echo "the second Core does not name the first: $(cat "$tmp/second.err")"; verdict=1
  fi

  kill -KILL "$first"; wait "$first" 2>/dev/null
  [[ -e "$tmp/run/instance.lock" ]] || { echo "the lock file the killed Core left is gone"; verdict=1; }
  "${export_env[@]}" "$tmp/yoke-core" 2>"$tmp/third.err" & third=$!
  sleep 1
  if ! kill -0 "$third" 2>/dev/null; then
    echo "the third Core did not serve the instance: $(cat "$tmp/third.err")"; verdict=1
  else
    kill -TERM "$third"; wait "$third" 2>/dev/null
  fi
  rm -rf "$tmp"
  return "$verdict"
}
