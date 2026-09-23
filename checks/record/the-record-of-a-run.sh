#!/usr/bin/env bash
# The checks described by the-record-of-a-run.std.md, one function per case.
# The two that need a run use the results this repository's own `test` verb leaves; the two that read
# a record build their results as a fixture, so a failing run is checked without one.

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
record_workflow="$root/.github/workflows/verify.yml"
record_script="$root/ci/record.sh"
record_results="$root/.results"

# std: yoke:the-record-of-a-run.01
check_a_run_leaves_its_results() {
  [[ -d "$record_results" ]] || { echo "the run left no results in .results"; return 1; }
  local each
  for each in checks.txt go.json started finished; do
    [[ -s "$record_results/$each" ]] || { echo "the run left no $each"; return 1; }
  done
  grep -qE '^(pass|FAIL)  ' "$record_results/checks.txt" \
    || { echo "checks.txt holds no result a check wrote"; return 1; }
  grep -q '"Action"' "$record_results/go.json" \
    || { echo "go.json is not the runner's own output"; return 1; }
  local each_instant
  for each_instant in started finished; do
    grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:]+Z$' "$record_results/$each_instant" \
      || { echo "$each_instant is not an instant in UTC: $(cat "$record_results/$each_instant")"; return 1; }
  done
}

# A directory of results, with the verdict given for one case of this repository's own descriptions.
record_fixture() {
  local directory="$1" verdict="$2" id="$3"
  mkdir -p "$directory"
  if [[ "$verdict" == pass ]]; then
    printf 'pass  %s\n' "$id" > "$directory/checks.txt"
  else
    printf 'FAIL  %s — the fixture says so\n' "$id" > "$directory/checks.txt"
  fi
  : > "$directory/go.json"
  date -u +%Y-%m-%dT%H:%M:%SZ > "$directory/started"
  date -u +%Y-%m-%dT%H:%M:%SZ > "$directory/finished"
}

# std: yoke:the-record-of-a-run.02
check_the_record_is_assembled_from_them() {
  [[ -x "$record_script" ]] || { echo "no ci/record.sh"; return 1; }
  command -v python3 >/dev/null || { echo "no python3"; return 1; }

  local tmp written
  tmp="$(mktemp -d)"
  record_fixture "$tmp" pass "yoke:verbs-and-licence.01"
  if ! written="$("$record_script" "$tmp" 2>&1)"; then
    echo "the script refused the results: $written"; rm -rf "$tmp"; return 1
  fi
  rm -rf "$tmp"

  local field
  for field in schema repository commit level tier environment started finished ran state blocks cases; do
    python3 -c "import json,sys; d=json.loads(sys.argv[1]); sys.exit(0 if '$field' in d else 1)" "$written" \
      || { echo "the record carries no $field"; return 1; }
  done
  python3 - "$written" <<'PY' || return 1
import json, sys
record = json.loads(sys.argv[1])
if record["repository"] != "yoke":
    raise SystemExit(f'the record names the repository {record["repository"]!r}')
if not record["commit"]:
    raise SystemExit("the record names no commit")
if record["level"] != "L1" or record["tier"] != "reference":
    raise SystemExit(f'the record is of {record["level"]} {record["tier"]}')
if not record["environment"] or not record["ran"]:
    raise SystemExit("the record names no environment or no artifact")
if not record["cases"]:
    raise SystemExit("the record holds no case")
PY
}

# std: yoke:the-record-of-a-run.03
check_a_failing_run_is_recorded_as_one() {
  [[ -x "$record_script" ]] || { echo "no ci/record.sh"; return 1; }
  local tmp written
  tmp="$(mktemp -d)"
  record_fixture "$tmp" fail "yoke:verbs-and-licence.01"
  written="$("$record_script" "$tmp" 2>&1)" || { echo "the script refused the results: $written"; rm -rf "$tmp"; return 1; }
  rm -rf "$tmp"

  python3 - "$written" <<'PY' || return 1
import json, sys
record = json.loads(sys.argv[1])
if record["state"] != "failed":
    raise SystemExit(f'the state is {record["state"]!r}, and a case failed')
if not record["blocks"]:
    raise SystemExit("blocks is false, and the case that failed is blocking")
failed = [c for c in record["cases"] if c["result"] == "fail"]
if not failed:
    raise SystemExit("no case is recorded as failing")
PY
}

# std: yoke:the-record-of-a-run.04
check_the_workflow_hands_it_over() {
  [[ -f "$record_workflow" ]] || { echo "no workflow"; return 1; }
  grep -qE '^[[:space:]]*- run: ci/record\.sh' "$record_workflow" \
    || { echo "no step assembles the record with a script under ci/"; return 1; }
  grep -qE 'upload-artifact' "$record_workflow" || { echo "the record is not handed over"; return 1; }
  local always
  always="$(grep -cE '^[[:space:]]*if: always\(\)' "$record_workflow")"
  (( always >= 2 )) || { echo "the record's steps do not run whatever the verbs decided"; return 1; }
}

# std: yoke:the-record-of-a-run.05
check_nothing_a_run_writes_enters_the_tree() {
  [[ -f "$root/.gitignore" ]] || { echo "no .gitignore"; return 1; }
  grep -qE '^\.results' "$root/.gitignore" || { echo ".gitignore does not ignore the results"; return 1; }
  (cd "$root" && git check-ignore -q .results) || { echo "the results are not ignored"; return 1; }
  local seen
  seen="$(cd "$root" && git status --porcelain | grep -F '.results' || true)"
  [[ -z "$seen" ]] || { echo "a run left the results in the tree: $seen"; return 1; }
}
