#!/usr/bin/env bash
# Prints the paths a change touched, one per line, against the base commit given as the first argument.
# With no usable base — a new branch, a repository's first push — every tracked path counts as changed.
set -euo pipefail

base="${1:-}"
if [[ -z "$base" || "$base" =~ ^0+$ ]] || ! git cat-file -e "${base}^{commit}" 2>/dev/null; then
  git ls-files
else
  git diff --name-only "$base" HEAD
fi
