#!/usr/bin/env bash
# The checks described by the-install-script.std.md, one function per case.
# A release is made of stand-in programs and served from a directory as file:// URLs, so nothing here
# reaches the network.

is_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
is_script="$is_root/install/yoke-install.sh"

# is_release <dir> <version>: a release of stand-in programs with the packaging inputs, both architectures.
is_release() {
  local where="$1/v$2" arch stage
  mkdir -p "$where"
  for arch in amd64 arm64; do
    stage="$(mktemp -d)"
    mkdir -p "$stage/packaging"
    for program in yoke yoke-core yoke-guardian yoke-auth; do
      printf '#!/bin/sh\necho %s %s\n' "$program" "$2" > "$stage/$program" && chmod 0755 "$stage/$program"
    done
    cp "$is_root"/packaging/{yoke.service,yoke-guardian.service,core.yaml,yoke.tmpfiles,yoke.sysusers} "$stage/packaging/"
    tar -czf "$where/yoke-$2-linux-$arch.tar.gz" -C "$stage" yoke yoke-core yoke-guardian yoke-auth packaging
    rm -rf "$stage" && stage="$(mktemp -d)"
    for program in yoke-conformance yoke-core yoke-compose; do
      printf '#!/bin/sh\necho %s %s\n' "$program" "$2" > "$stage/$program" && chmod 0755 "$stage/$program"
    done
    tar -czf "$where/yoke-conformance-$2-linux-$arch.tar.gz" -C "$stage" yoke-conformance yoke-core yoke-compose
    rm -rf "$stage"
  done
}

# is_digest <file>
is_digest() { sha256sum "$1" | cut -d' ' -f1; }

# is_published <dir> <version>: the script as a release writes it, carrying that release and its digests.
is_published() {
  local file digests=""
  for file in "$1/v$2"/*.tar.gz; do
    digests="$digests$(is_digest "$file")  $(basename "$file")"$'\n'
  done
  awk -v version="$2" -v digests="$digests" '
    $0 == "YOKE_RELEASE=\"\"" { print "YOKE_RELEASE=\"" version "\""; next }
    $0 == "YOKE_DIGESTS=\"\"" { printf "YOKE_DIGESTS=\"\n%s\"\n", digests; next }
    { print }' "$is_script" > "$1/yoke-install.sh"
}

# is_manifest <dir> <version>: the manifest's lines for that release, in their fixed shape.
is_manifest() {
  local arch name
  for arch in amd64 arm64; do
    for name in yoke yoke-conformance; do
      printf '{"line":"publication","published":"%s-linux-%s","version":"v%s","commit":"0","digests":["sha256:%s"],"where":"x","authenticated":"x","licence":"Apache-2.0","notices":[],"day":"2026-10-30"}\n' \
        "$name" "$arch" "$2" "$(is_digest "$1/v$2/$name-$2-linux-$arch.tar.gz")" >> "$1/manifest.jsonl"
    done
  done
}

# is_run <dir> <home> <arguments>…: the script, published from <dir>, run in <home>.
is_run() {
  local from="$1" home="$2"
  shift 2
  HOME="$home" XDG_CONFIG_HOME="$home/.config" XDG_STATE_HOME="$home/.local/state" XDG_RUNTIME_DIR="$home/run" \
    YOKE_INSTALL_RELEASES="file://$from" \
    YOKE_INSTALL_MANIFEST="file://$from/manifest.jsonl" sh "$from/yoke-install.sh" "$@"
}

# std: yoke:the-install-script.01
check_a_published_copy_obtains_its_own_release() {
  local from home
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  is_run "$from" "$home" --arch arm64 --dir "$home/kept" > /dev/null || { echo "the script refused"; return 1; }
  cmp -s "$home/kept/yoke-0.1.0-linux-arm64.tar.gz" "$from/v0.1.0/yoke-0.1.0-linux-arm64.tar.gz" \
    && cmp -s "$home/kept/yoke-conformance-0.1.0-linux-arm64.tar.gz" "$from/v0.1.0/yoke-conformance-0.1.0-linux-arm64.tar.gz" \
    || { echo "kept: $(ls "$home/kept")"; return 1; }
}

# std: yoke:the-install-script.02
check_another_release_is_checked_against_the_manifest() {
  local from home said
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  is_release "$from" 0.0.9 && is_manifest "$from" 0.0.9
  said="$(is_run "$from" "$home" --version 0.0.9 --arch amd64 --dir "$home/kept" 2>&1)" || { echo "$said"; return 1; }
  grep -q "against the release manifest" <<< "$said" || { echo "$said"; return 1; }
  [[ -f "$home/kept/yoke-0.0.9-linux-amd64.tar.gz" && -f "$home/kept/yoke-conformance-0.0.9-linux-amd64.tar.gz" ]] \
    || { echo "kept: $(ls "$home/kept")"; return 1; }
}

# std: yoke:the-install-script.03
check_a_file_not_released_is_refused_and_nothing_kept() {
  local from home said
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  printf 'changed' >> "$from/v0.1.0/yoke-0.1.0-linux-amd64.tar.gz"
  if said="$(is_run "$from" "$home" --arch amd64 --dir "$home/kept" 2>&1)"; then
    echo "the script kept a changed file"; return 1
  fi
  grep -q "yoke-0.1.0-linux-amd64.tar.gz is not the file released" <<< "$said" || { echo "$said"; return 1; }
  [[ -z "$(ls -A "$home/kept" 2>/dev/null)" ]] || { echo "kept: $(ls "$home/kept")"; return 1; }
}

# std: yoke:the-install-script.04
check_a_release_nobody_published_is_refused() {
  local from home said
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  is_release "$from" 0.0.9 && is_manifest "$from" 0.0.9
  if said="$(is_run "$from" "$home" --version 0.0.8 --dir "$home/kept" 2>&1)"; then
    echo "an unpublished release was obtained"; return 1
  fi
  grep -q "lists no .* for 0.0.8" <<< "$said" || { echo "$said"; return 1; }
  cp "$is_script" "$from/yoke-install.sh"
  if said="$(is_run "$from" "$home" --dir "$home/kept" 2>&1)"; then
    echo "a copy with no release obtained one"; return 1
  fi
  grep -q "not published with a release; name one with --version" <<< "$said" || { echo "$said"; return 1; }
}

# std: yoke:the-install-script.05
check_a_machine_no_release_is_built_for_is_refused() {
  local from home said
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  if said="$(is_run "$from" "$home" --arch riscv64 --dir "$home/kept" 2>&1)"; then
    echo "riscv64 was accepted"; return 1
  fi
  grep -q "riscv64.*amd64 and arm64" <<< "$said" || { echo "$said"; return 1; }
  [[ ! -e "$home/kept" ]] || { echo "something was kept"; return 1; }
}

# std: yoke:the-install-script.06
check_a_different_file_is_replaced_only_when_asked() {
  local from home said
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  mkdir -p "$home/kept" && printf 'mine' > "$home/kept/yoke-0.1.0-linux-amd64.tar.gz"
  if said="$(is_run "$from" "$home" --arch amd64 --dir "$home/kept" 2>&1)"; then
    echo "a different file was replaced"; return 1
  fi
  grep -q "yoke-0.1.0-linux-amd64.tar.gz is there and differs; --force" <<< "$said" || { echo "$said"; return 1; }
  [[ "$(cat "$home/kept/yoke-0.1.0-linux-amd64.tar.gz")" == mine ]] || { echo "the file was touched"; return 1; }
  is_run "$from" "$home" --arch amd64 --dir "$home/kept" --force > /dev/null 2>&1 || { echo "--force refused"; return 1; }
  cmp -s "$home/kept/yoke-0.1.0-linux-amd64.tar.gz" "$from/v0.1.0/yoke-0.1.0-linux-amd64.tar.gz" || { echo "not replaced"; return 1; }
}

# std: yoke:the-install-script.07
check_installing_for_an_account_places_everything() {
  local from home other program said
  [[ "$(id -u)" -ne 0 ]] || { echo "run as root: the case needs an account that is not root"; return 1; }
  from="$(mktemp -d)" home="$(mktemp -d)" other="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  said="$(is_run "$from" "$home" --dir "$home/kept" --install account --bin "$home/bin" 2>&1)" || { echo "$said"; return 1; }
  for program in yoke yoke-core yoke-guardian yoke-auth yoke-compose; do
    [[ -x "$home/bin/$program" ]] || { echo "no $program"; return 1; }
  done
  [[ "$(stat -c %a "$home/yoke/apps")" == 2775 ]] || { echo "the install root is $(stat -c %a "$home/yoke/apps")"; return 1; }
  [[ "$(cat "$home/.config/yoke/manager.conf")" == "install_root: $home/yoke/apps" ]] || { echo "manager.conf says $(cat "$home/.config/yoke/manager.conf")"; return 1; }
  grep -qx "ExecStart=$home/bin/yoke-guardian --boot" "$home/.config/systemd/user/yoke-guardian.service" \
    || { echo "the unit runs $(grep ExecStart "$home/.config/systemd/user/yoke-guardian.service")"; return 1; }
  mkdir -p "$other/.config/yoke" && printf 'install_root: /somewhere\n' > "$other/.config/yoke/manager.conf"
  is_run "$from" "$other" --dir "$other/kept" --install account --bin "$other/bin" > /dev/null 2>&1 || { echo "the second home refused"; return 1; }
  [[ "$(cat "$other/.config/yoke/manager.conf")" == "install_root: /somewhere" ]] || { echo "an account's manager.conf was rewritten"; return 1; }
}

# std: yoke:the-install-script.08
check_installing_for_the_host_needs_root_first() {
  local from home said
  [[ "$(id -u)" -ne 0 ]] || { echo "run as root: the case needs an account that is not root"; return 1; }
  from="$(mktemp -d)" home="$(mktemp -d)"
  cp "$is_script" "$from/yoke-install.sh"
  if said="$(is_run "$from" "$home" --version 0.1.0 --install host --dir "$home/kept" 2>&1)"; then
    echo "a host install ran without root"; return 1
  fi
  grep -q "needs root; run it with sudo, or use --install account" <<< "$said" || { echo "$said"; return 1; }
}

# std: yoke:the-install-script.09
check_a_file_already_kept_is_used_not_downloaded() {
  local from home said
  [[ "$(id -u)" -ne 0 ]] || { echo "run as root: the case needs an account that is not root"; return 1; }
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  is_run "$from" "$home" --arch amd64 --dir "$home/kept" > /dev/null 2>&1 || { echo "the first run refused"; return 1; }
  mv "$from/v0.1.0" "$from/withdrawn"
  said="$(is_run "$from" "$home" --arch amd64 --dir "$home/kept" --install account --bin "$home/bin" 2>&1)" || { echo "$said"; return 1; }
  grep -q "using $home/kept/yoke-0.1.0-linux-amd64.tar.gz, already kept and checked" <<< "$said" || { echo "$said"; return 1; }
  [[ -x "$home/bin/yoke" ]] || { echo "nothing installed"; return 1; }
}

# std: yoke:the-install-script.10
check_a_directory_carried_without_network_installs() {
  local from home carried program said
  [[ "$(id -u)" -ne 0 ]] || { echo "run as root: the case needs an account that is not root"; return 1; }
  from="$(mktemp -d)" home="$(mktemp -d)" carried="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  cp "$from/yoke-install.sh" "$from/v0.1.0/yoke-0.1.0-linux-arm64.tar.gz" "$carried/"
  said="$(HOME="$home" XDG_CONFIG_HOME="$home/.config" XDG_STATE_HOME="$home/.local/state" XDG_RUNTIME_DIR="$home/run" \
    YOKE_INSTALL_RELEASES="file:///nowhere" \
    YOKE_INSTALL_MANIFEST="file:///nowhere/manifest.jsonl" \
    sh "$carried/yoke-install.sh" --arch arm64 --dir "$carried" --runtime-only --install account --bin "$home/bin" 2>&1)" \
    || { echo "$said"; return 1; }
  for program in yoke yoke-core yoke-guardian yoke-auth; do
    [[ -x "$home/bin/$program" ]] || { echo "no $program"; return 1; }
  done
  [[ ! -e "$home/bin/yoke-compose" ]] || { echo "the composer was installed"; return 1; }
}

# std: yoke:the-install-script.11
check_an_earlier_installation_is_replaced_only_when_upgrading() {
  local from home said
  [[ "$(id -u)" -ne 0 ]] || { echo "run as root: the case needs an account that is not root"; return 1; }
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  is_run "$from" "$home" --dir "$home/kept" --install account --bin "$home/bin" > /dev/null 2>&1 || { echo "0.1.0 did not install"; return 1; }
  is_release "$from" 0.2.0 && is_published "$from" 0.2.0
  if said="$(is_run "$from" "$home" --dir "$home/kept" --install account --bin "$home/bin" 2>&1)"; then
    echo "an earlier installation was replaced without --upgrade"; return 1
  fi
  grep -q "differs, from an earlier installation; --upgrade replaces it" <<< "$said" || { echo "$said"; return 1; }
  [[ "$("$home/bin/yoke")" == "yoke 0.1.0" ]] || { echo "the programs were touched"; return 1; }
  said="$(is_run "$from" "$home" --dir "$home/kept" --install account --bin "$home/bin" --upgrade 2>&1)" || { echo "$said"; return 1; }
  [[ "$("$home/bin/yoke")" == "yoke 0.2.0" ]] || { echo "the programs are $("$home/bin/yoke")"; return 1; }
  grep -q "Start again each instance you stopped" <<< "$said" || { echo "$said"; return 1; }
  [[ "$(cat "$home/.config/yoke/manager.conf")" == "install_root: $home/yoke/apps" ]] || { echo "manager.conf was rewritten"; return 1; }
}

# std: yoke:the-install-script.12
check_nothing_changes_while_a_core_runs() {
  local from home said core status=0
  [[ "$(id -u)" -ne 0 ]] || { echo "run as root: the case needs an account that is not root"; return 1; }
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  is_run "$from" "$home" --dir "$home/kept" --install account --bin "$home/bin" > /dev/null 2>&1 || { echo "0.1.0 did not install"; return 1; }
  # A Core of this installation: a process executing the installed yoke-core.
  cp "$(command -v sleep)" "$home/bin/yoke-core"
  "$home/bin/yoke-core" 60 &
  core=$!
  sleep 0.2
  if said="$(is_run "$from" "$home" --dir "$home/kept" --install account --bin "$home/bin" --upgrade 2>&1)"; then
    echo "upgraded while a Core ran"; status=1
  elif ! grep -q "an instance of this account is running; stop each one first .*yoke instance stop" <<< "$said"; then
    echo "$said"; status=1
  fi
  if [[ "$status" -eq 0 ]]; then
    if said="$(is_run "$from" "$home" --uninstall account --bin "$home/bin" 2>&1)"; then
      echo "removed while a Core ran"; status=1
    elif ! grep -q "yoke instance stop" <<< "$said"; then
      echo "$said"; status=1
    fi
  fi
  kill "$core" 2> /dev/null
  wait "$core" 2> /dev/null
  [[ "$status" -eq 0 ]] || return 1
  [[ -x "$home/bin/yoke" ]] || { echo "a program was removed"; return 1; }
}

# std: yoke:the-install-script.13
check_removing_from_an_account_keeps_its_data_until_purged() {
  local from home said
  [[ "$(id -u)" -ne 0 ]] || { echo "run as root: the case needs an account that is not root"; return 1; }
  from="$(mktemp -d)" home="$(mktemp -d)"
  is_release "$from" 0.1.0 && is_published "$from" 0.1.0
  is_run "$from" "$home" --dir "$home/kept" --install account --bin "$home/bin" > /dev/null 2>&1 || { echo "0.1.0 did not install"; return 1; }
  mkdir -p "$home/.local/state/yoke/instances/a" "$home/yoke/apps/com.example.app"
  if said="$(is_run "$from" "$home" --uninstall account --purge --bin "$home/bin" 2>&1)"; then
    echo "purged with an instance left"; return 1
  fi
  grep -q "still has instances; .*yoke instance remove" <<< "$said" || { echo "$said"; return 1; }
  [[ -x "$home/bin/yoke" ]] || { echo "a refused purge removed a program"; return 1; }
  said="$(is_run "$from" "$home" --uninstall account --bin "$home/bin" 2>&1)" || { echo "$said"; return 1; }
  [[ ! -e "$home/bin/yoke" && ! -e "$home/bin/yoke-core" && ! -e "$home/.config/systemd/user/yoke-guardian.service" ]] \
    || { echo "a program or the unit is left"; return 1; }
  [[ -f "$home/.config/yoke/manager.conf" && -d "$home/yoke/apps/com.example.app" && -d "$home/.local/state/yoke/instances/a" ]] \
    || { echo "the data was not kept"; return 1; }
  rmdir "$home/.local/state/yoke/instances/a" "$home/yoke/apps/com.example.app"
  said="$(is_run "$from" "$home" --uninstall account --purge --bin "$home/bin" 2>&1)" || { echo "$said"; return 1; }
  [[ ! -e "$home/.config/yoke" && ! -e "$home/yoke/apps" && ! -e "$home/.local/state/yoke" ]] || { echo "the purge left data"; return 1; }
}

# std: yoke:the-install-script.14
check_removing_from_the_host_needs_root() {
  local from home said
  [[ "$(id -u)" -ne 0 ]] || { echo "run as root: the case needs an account that is not root"; return 1; }
  from="$(mktemp -d)" home="$(mktemp -d)"
  cp "$is_script" "$from/yoke-install.sh"
  if said="$(is_run "$from" "$home" --uninstall host 2>&1)"; then
    echo "a host removal ran without root"; return 1
  fi
  grep -q "needs root; run it with sudo" <<< "$said" || { echo "$said"; return 1; }
}
