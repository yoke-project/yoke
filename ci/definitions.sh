#!/usr/bin/env bash
# The definitions' tools. The Go and the checks run from source through the module proxy, needing Go and
# `just`; the crate's Rust and the wheel's Python are generated, tested and packaged in the images below,
# pinned by digest, needing the container engine this repository's own checks already need. The Go
# plugins' versions are in proto/buf.gen.yaml; buf's is here; the Rust generator's are in its lockfile and
# the Python generator's in its requirements.
#
# Usage: definitions.sh check                         build, lint, format and the two version statements
#        definitions.sh generate <dir>                the Go the definitions generate, into <dir>, relative to proto/
#        definitions.sh generate-rust <dir>           the crate's Rust, into <dir>, relative to proto/
#        definitions.sh generate-python <dir>         the wheel's Python, into <dir>, relative to proto/
#        definitions.sh test-rust | test-python       build the package and run its tests
#        definitions.sh package <version> <dir>       the crate and the wheel at <version>, into <dir>, relative to proto/
#        definitions.sh publish-crate <version>       publish the crate at <version>, under CARGO_REGISTRY_TOKEN
set -euo pipefail

buf=(go run github.com/bufbuild/buf/cmd/buf@v1.73.0)
rust_image=docker.io/library/rust:1.94.1-slim-bookworm@sha256:cf9dd0ec73e75f827fe59123fff9dc65af1a1c8363c3c31ee8d7f8ad0b6a5fb2
python_image=docker.io/library/python:3.14.7-slim-bookworm@sha256:82bc3c539b8813ada9d68c63b40158fa002f7f33de9bf3312a3dfdc0620dff56

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo/proto"

# Runs a command in an image, the repository mounted read-only at /src and <out> writable at /out, as the
# invoking user so what it writes is theirs.
contained() {
  local image="$1" out="$2"
  shift 2
  local engine
  engine="$(command -v podman || command -v docker)" || { echo "definitions: no container engine: podman or docker is needed" >&2; exit 1; }
  local as=(--user "$(id -u):$(id -g)")
  [[ "$(basename "$engine")" == podman ]] && as=(--userns=keep-id)
  "$engine" run --rm "${as[@]}" -e HOME=/tmp -e CARGO_HOME=/tmp/cargo -e CARGO_TARGET_DIR=/tmp/target \
    -e PIP_DISABLE_PIP_VERSION_CHECK=1 -e PIP_NO_CACHE_DIR=1 -e CARGO_REGISTRY_TOKEN \
    -v "$repo:/src:ro" -v "$out:/out" "$image" bash -euo pipefail -c "$*"
}

# The definitions' files, relative to proto/, in a stable order.
protos() { find yoke -name '*.proto' | LC_ALL=C sort | tr '\n' ' '; }

# A package's project copied where it can be written, with the licence and the notice beside it and the
# version stated — the step every build of a package starts from.
stage() { # <project> <version>
  echo "cp -R /src/proto/$1 /tmp/$1 && cp /src/LICENSE /src/NOTICE /tmp/$1/ &&
        sed -i '0,/^version = \"0.0.0\"/s//version = \"$2\"/' /tmp/$1/$( [[ $1 == rust ]] && echo Cargo.toml || echo pyproject.toml )"
}

# A fresh environment with pip, at /tmp/env.
environment() { echo "python -m venv /tmp/env"; }

out_dir() { mkdir -p "$1" && cd "$1" && pwd; }

case "${1:-}" in
  check)
    "${buf[@]}" build
    "${buf[@]}" lint
    "${buf[@]}" format --diff --exit-code
    go run ./internal/contracts .
    echo "definitions: they build, lint clean, are formatted, and every contract states its path's integer"
    ;;
  generate)
    "${buf[@]}" generate --output "${2:?usage: definitions.sh generate <dir>}"
    ;;
  generate-rust)
    out="$(out_dir "${2:?usage: definitions.sh generate-rust <dir>}")"
    contained "$rust_image" "$out" \
      "cd /src/proto && cargo run -q --locked --release --manifest-path generators/rust/Cargo.toml -- /src/proto /out $(protos)"
    ;;
  generate-python)
    out="$(out_dir "${2:?usage: definitions.sh generate-python <dir>}")"
    contained "$python_image" "$out" \
      "$(environment) && /tmp/env/bin/pip install -q --no-deps --require-hashes -r /src/proto/generators/python/requirements.txt &&
       cd /src/proto && /tmp/env/bin/python -m grpc_tools.protoc -I . --python_out=/out --pyi_out=/out --grpc_python_out=/out $(protos)"
    ;;
  test-rust)
    contained "$rust_image" "$(mktemp -d)" "$(stage rust 0.0.0) && cd /tmp/rust && cargo test -q --locked"
    ;;
  test-python)
    contained "$python_image" "$(mktemp -d)" \
      "$(stage python 0.0.0) && $(environment) && /tmp/env/bin/pip wheel -q --no-deps -w /tmp/dist /tmp/python &&
       /tmp/env/bin/pip install -q /tmp/dist/*.whl && cd /tmp/python/tests && /tmp/env/bin/python -m unittest -q"
    ;;
  package)
    version="${2:?usage: definitions.sh package <version> <dir>}"
    [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "definitions: $version is not a version: it is written X.Y.Z" >&2; exit 2; }
    out="$(out_dir "${3:?usage: definitions.sh package <version> <dir>}")"
    contained "$rust_image" "$out" "$(stage rust "$version") && cd /tmp/rust && cargo package -q --allow-dirty &&
      cp /tmp/target/package/yoke-proto-$version.crate /out/"
    contained "$python_image" "$out" "$(stage python "$version") && $(environment) &&
      /tmp/env/bin/pip wheel -q --no-deps -w /out /tmp/python"
    ;;
  publish-crate)
    version="${2:?usage: definitions.sh publish-crate <version>}"
    [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "definitions: $version is not a version: it is written X.Y.Z" >&2; exit 2; }
    [[ -n "${CARGO_REGISTRY_TOKEN:-}" ]] || { echo "definitions: no crates.io credential: CARGO_REGISTRY_TOKEN is empty" >&2; exit 1; }
    contained "$rust_image" "$(mktemp -d)" "$(stage rust "$version") && cd /tmp/rust && cargo publish -q --allow-dirty"
    ;;
  *)
    echo "usage: definitions.sh check | generate <dir> | generate-rust <dir> | generate-python <dir> | test-rust | test-python | package <version> <dir> | publish-crate <version>" >&2
    exit 2
    ;;
esac
