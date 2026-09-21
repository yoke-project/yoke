# The six verbs every repository defines (prj_structure/95 §The verbs).
# A verb with nothing to do says so in one line, so a fan-out can tell a gap from a statement.

# Build this repository's codebase.
build:
    @go build ./... && echo "build: every package builds"

# Run this repository's own checks, with no sibling present.
test:
    #!/usr/bin/env bash
    set -euo pipefail
    bash checks/run.sh
    go test ./...
    go run ./cmd/yoke-verify descriptions --repository yoke . > /dev/null
    go run ./cmd/yoke-verify markers --repository yoke . > /dev/null
    echo "test: every description holds its form, and every case has exactly one test"

# This repository's static checks.
lint:
    #!/usr/bin/env bash
    set -euo pipefail
    bash -n checks/run.sh checks/*/*.sh ci/*.sh
    go vet ./...
    echo "lint: every shell script parses and go vet is clean"

# Fail, naming each file, when the tree is not formatted.
fmt:
    #!/usr/bin/env bash
    set -euo pipefail
    files="$(find . -name '*.go' -not -path './.git/*' -print0 | xargs -0 -r gofmt -l)"
    if [[ -n "$files" ]]; then printf 'fmt: not formatted:\n%s\n' "$files"; exit 1; fi
    echo "fmt: every Go file is formatted"

# Verify the toolchain against the floor the workspace's fan-out passes (466).
develop floor="":
    #!/usr/bin/env bash
    set -euo pipefail
    found="$(just --version | awk '{print $2}')"
    if [[ -z "{{floor}}" ]]; then
        echo "develop: no floor given, so none verified — the workspace passes it (466); found just $found"
        exit 0
    fi
    if ! [[ "{{floor}}" =~ ^[0-9]+(\.[0-9]+)*$ ]]; then
        echo "develop: '{{floor}}' is not a version; pass it as \`just develop 1.58.0\`"
        exit 1
    fi
    lowest="$(printf '%s\n%s\n' "{{floor}}" "$found" | sort -V | head -n 1)"
    if [[ "$lowest" != "{{floor}}" ]]; then
        echo "develop: just {{floor}} or newer is needed; found just $found"
        exit 1
    fi
    echo "develop: just $found meets the floor {{floor}}"

# Publish into this repository's ecosystems, one manifest line per publication (393).
release:
    @echo "release: nothing to publish yet — the release verb arrives with yoke-project/yoke#20"
