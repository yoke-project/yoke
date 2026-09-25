#!/usr/bin/env bash
# The checks described by the-definitions-module.std.md, one function per case.
# The consumer is built offline, from the module cache this repository's own build fills.

dm_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dm_module=github.com/yoke-project/yoke/proto

dm_start() { [[ -f "$dm_root/proto/go.mod" ]] || { echo "the definitions' directory is not a module"; return 1; }; }

# std: yoke:the-definitions-module.01
check_the_definitions_directory_is_a_module() {
  dm_start || return 1
  local declared outside
  declared="$(cd "$dm_root/proto" && go list -m 2>&1)"
  [[ "$declared" == "$dm_module" ]] || { echo "the module is $declared, not $dm_module"; return 1; }
  outside="$(grep -rhoE 'go_package = "[^";]+' "$dm_root/proto" --include='*.proto' --exclude-dir=testdata \
    | sed 's/go_package = "//' | grep -v "^$dm_module/" || true)"
  [[ -z "$outside" ]] || { echo "a generated package outside the module: $outside"; return 1; }
}

# std: yoke:the-definitions-module.02
check_the_root_module_holds_none_of_the_definitions() {
  dm_start || return 1
  local listed
  listed="$(cd "$dm_root" && GOWORK=off go list ./... 2>&1)" || { echo "the root module does not list: $listed"; return 1; }
  if grep -q "^github.com/yoke-project/yoke/proto" <<<"$listed"; then
    echo "the root module holds $(grep "^github.com/yoke-project/yoke/proto" <<<"$listed" | head -3 | tr '\n' ' ')"
    return 1
  fi
}

# std: yoke:the-definitions-module.03
check_a_consumer_builds_with_the_module_alone() {
  dm_start || return 1
  local tmp said verdict=0; tmp="$(mktemp -d)"
  cp -R "$dm_root/proto" "$tmp/definitions"
  mkdir -p "$tmp/consumer"
  cat > "$tmp/consumer/go.mod" <<MOD
module example.org/consumer

go 1.26

require $dm_module v0.1.0

replace $dm_module => ../definitions
MOD
  cat > "$tmp/consumer/main.go" <<'GO'
package main

import (
	"fmt"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"
)

func main() {
	envelope := &pluginv1.Envelope{MessageId: "m-1", SessionId: "s-1"}
	fmt.Println(envelope.GetMessageId())
}
GO
  if ! said="$(cd "$tmp/consumer" && GOWORK=off GOFLAGS=-mod=mod GOPROXY=off go build ./... 2>&1)"; then
    echo "the consumer does not build with the module alone: ${said: -400}"; verdict=1
  fi
  rm -rf "$tmp"
  return "$verdict"
}

# std: yoke:the-definitions-module.04
check_the_verbs_check_both_modules() {
  local verb recipe
  for verb in build test lint; do
    recipe="$(awk -v verb="$verb" '
      $0 ~ "^" verb "( |:)" { inside = 1; next }
      inside && /^[^ \t#]/ { inside = 0 }
      inside { print }' "$dm_root/justfile")"
    grep -qE 'go (build|test|vet)' <<<"$recipe" || { echo "$verb runs no Go"; return 1; }
    grep -qE '(-C proto|cd proto).*go (build|test|vet)|go (build|test|vet).*-C proto' <<<"$recipe" \
      || { echo "$verb runs nothing in the definitions module"; return 1; }
  done
}
