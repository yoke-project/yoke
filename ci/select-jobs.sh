#!/usr/bin/env bash
# The path filter: reads the paths a change touched, one per line, and prints the jobs it selects.
# A job runs when its inputs moved; the suite and the version comparison are never skipped when the
# definitions move (prj_structure/95 §Continuous integration).
set -euo pipefail

verify=0
definitions=0
while IFS= read -r path; do
  [[ -n "$path" ]] || continue
  case "$path" in
    checks/*) verify=1 ;;
    definitions/*) verify=1; definitions=1 ;;
    *.md | LICENSE | NOTICE) ;;
    *) verify=1 ;;
  esac
done

if (( verify )); then echo verify; fi
if (( definitions )); then
  echo suite
  echo version-comparison
fi
