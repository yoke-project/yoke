#!/bin/sh
# Obtains a release of Yoke, checks every file against its published digest, and on request installs it
# for this account or for the whole host, upgrades an earlier installation, or removes one. It is published with each release as yoke-install.sh, and read
# by `sh`:
#
#   curl --proto '=https' --tlsv1.2 -sSf https://github.com/yoke-project/yoke/releases/latest/download/yoke-install.sh | sh
#
# Arguments follow `sh -s --`. It writes nothing outside the directories it names, and stops at the first
# thing it cannot do, saying what and why. See https://docs.yoke-project.dev/setting-up/linux-host/
set -eu

# The release this copy was published with, and the digests of that release's files, one
# "<sha256>  <file>" per line. Both are written in when the release is published; a copy taken from the
# repository has neither, and needs --version.
YOKE_RELEASE=""
YOKE_DIGESTS=""

releases="${YOKE_INSTALL_RELEASES:-https://github.com/yoke-project/yoke/releases/download}"
manifest="${YOKE_INSTALL_MANIFEST:-https://raw.githubusercontent.com/yoke-project/yoke/main/releases/manifest.jsonl}"

usage() {
  cat <<'EOF'
yoke-install.sh: obtain a release of Yoke, check it, and install it on request.

  --version <x.y.z>   the release to obtain; by default, the one this script was published with
  --arch <arch>       amd64 or arm64; by default, this machine's
  --dir <path>        where the release files are kept, and found again; by default ~/yoke-release
  --runtime-only      obtain the programs a host runs, without the workstation's composer and suite
  --install account   install for this account: programs in ~/.local/bin, an install root of its own
  --install host      install for the whole host, as root: programs in /usr/bin and the packaging inputs
  --bin <path>        with --install account, where the programs go; by default ~/.local/bin
  --at-boot           with --install account, bring this account's instances back when the host boots
  --upgrade           with --install, replace the programs and units of an earlier installation
  --uninstall account remove what --install account placed; data and configuration are kept
  --uninstall host    remove what --install host placed, as root; data and configuration are kept
  --purge             with --uninstall, remove the data and the configuration as well
  --force             replace files that are already there and differ
  --help              this text
EOF
}

say() { printf '%s\n' "$*"; }
warn() { printf 'yoke-install: warning: %s\n' "$*" >&2; }
refuse() { printf 'yoke-install: %s\n' "$*" >&2; exit 1; }
has() { command -v "$1" >/dev/null 2>&1; }

version="" arch="" dir="${HOME:-}/yoke-release" runtime_only=0 install="" bin="${HOME:-}/.local/bin" at_boot=0 force=0
upgrade=0 uninstall="" purge=0
while [ $# -gt 0 ]; do
  case "$1" in
    --version) [ $# -ge 2 ] || refuse "--version needs a value"; version="${2#v}"; shift 2 ;;
    --arch) [ $# -ge 2 ] || refuse "--arch needs a value"; arch="$2"; shift 2 ;;
    --dir) [ $# -ge 2 ] || refuse "--dir needs a value"; dir="$2"; shift 2 ;;
    --runtime-only) runtime_only=1; shift ;;
    --install) [ $# -ge 2 ] || refuse "--install needs account or host"; install="$2"; shift 2 ;;
    --bin) [ $# -ge 2 ] || refuse "--bin needs a value"; bin="$2"; shift 2 ;;
    --at-boot) at_boot=1; shift ;;
    --upgrade) upgrade=1; shift ;;
    --uninstall) [ $# -ge 2 ] || refuse "--uninstall needs account or host"; uninstall="$2"; shift 2 ;;
    --purge) purge=1; shift ;;
    --force) force=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) usage >&2; refuse "unknown argument: $1" ;;
  esac
done
case "$install" in
  ""|account|host) ;;
  *) refuse "--install takes account or host, not $install" ;;
esac
[ "$at_boot" -eq 0 ] || [ "$install" = account ] || refuse "--at-boot goes with --install account"
[ "$upgrade" -eq 0 ] || [ -n "$install" ] || refuse "--upgrade goes with --install account or --install host"
case "$uninstall" in
  ""|account|host) ;;
  *) refuse "--uninstall takes account or host, not $uninstall" ;;
esac
[ -z "$uninstall" ] || [ -z "$install" ] || refuse "--install and --uninstall are two runs, not one"
[ "$purge" -eq 0 ] || [ -n "$uninstall" ] || refuse "--purge goes with --uninstall"

# running_here <scope>: whether Yoke is running where the scope reaches — a Core this account runs from
# the programs this installation placed, or any Core and the service form's unit on this host.
# It reads /proc itself, so that a host without pgrep is not taken for one where nothing runs.
running_here() {
  for proc in /proc/[0-9]*; do
    [ "$(cat "$proc/comm" 2>/dev/null)" = yoke-core ] || continue
    [ "$1" = host ] && return 0
    case "$(readlink "$proc/exe" 2>/dev/null)" in
      "$bin/yoke-core"|"$bin/yoke-core (deleted)") return 0 ;;
    esac
  done
  [ "$1" = host ] && has systemctl && systemctl is-active --quiet yoke.service 2>/dev/null
}
stop_first() { # <scope>
  if [ "$1" = account ]; then
    refuse "an instance of this account is running; stop each one first (yoke instance list, then yoke instance stop <name>), and start them again afterwards"
  fi
  refuse "Yoke is running on this host; stop the service form (yoke service stop) and every account's instances first"
}
# Who may install what is settled before anything is downloaded.
if [ "$install" = account ]; then
  [ "$(id -u)" -ne 0 ] || refuse "--install account installs for the account running it, and this is root; --install host installs for the whole host"
fi
if [ "$install" = host ]; then
  [ "$(id -u)" -eq 0 ] || refuse "--install host installs for the whole host and needs root; run it with sudo, or use --install account"
  { command -v systemd-sysusers && command -v systemd-tmpfiles; } >/dev/null 2>&1 \
    || refuse "--install host needs systemd's sysusers and tmpfiles, which this host does not have"
fi
if [ "$upgrade" -eq 1 ] && running_here "$install"; then stop_first "$install"; fi

# Removing: what an installation placed, and with --purge what Yoke kept, never what somebody else put
# there. It downloads nothing.
if [ -n "$uninstall" ]; then
  if [ "$uninstall" = account ]; then
    [ "$(id -u)" -ne 0 ] || refuse "--uninstall account removes this account's installation, and this is root; --uninstall host removes the host's"
  else
    [ "$(id -u)" -eq 0 ] || refuse "--uninstall host removes the host's installation and needs root; run it with sudo"
  fi
  if running_here "$uninstall"; then stop_first "$uninstall"; fi
  if [ "$uninstall" = account ]; then
    config="${XDG_CONFIG_HOME:-$HOME/.config}"
    state="${XDG_STATE_HOME:-$HOME/.local/state}/yoke"
    if [ "$purge" -eq 1 ]; then
      # An instance or an application is somebody's data, removed by its own verb and never by this script.
      [ -z "$(ls -A "$state/instances" 2>/dev/null)" ] \
        || refuse "this account still has instances; remove each one first (yoke instance list, then yoke instance remove <name>)"
      root="$(sed -n 's/^install_root:[[:space:]]*//p' "$config/yoke/manager.conf" 2>/dev/null | head -n 1)"
      if [ -n "$root" ] && [ -n "$(ls -A "$root" 2>/dev/null)" ]; then
        refuse "the install root $root still holds applications; remove each one first (yoke app list, then yoke app remove <id>)"
      fi
    fi
    if has systemctl && [ -e "$config/systemd/user/yoke-guardian.service" ]; then
      systemctl --user disable yoke-guardian.service >/dev/null 2>&1 || true
    fi
    for program in yoke yoke-core yoke-guardian yoke-auth yoke-compose; do
      [ ! -e "$bin/$program" ] || { rm -f "$bin/$program"; say "removed $bin/$program"; }
    done
    [ ! -e "$config/systemd/user/yoke-guardian.service" ] || { rm -f "$config/systemd/user/yoke-guardian.service"; say "removed the guardian's user unit"; }
    if [ "$purge" -eq 1 ]; then
      [ -z "$root" ] || [ ! -d "$root" ] || { rmdir "$root" && say "removed the install root $root"; }
      [ ! -e "$config/yoke" ] || { rm -rf "$config/yoke"; say "removed $config/yoke"; }
      [ ! -e "$state" ] || { rm -rf "$state"; say "removed $state"; }
      [ -z "${XDG_RUNTIME_DIR:-}" ] || [ ! -e "$XDG_RUNTIME_DIR/yoke" ] || { rm -rf "$XDG_RUNTIME_DIR/yoke"; say "removed $XDG_RUNTIME_DIR/yoke"; }
      say "Yoke is removed from this account, with its data."
    else
      say "Yoke is removed from this account; its configuration, install root and instances' state are kept (--purge removes them)."
    fi
    exit 0
  fi
  if [ "$purge" -eq 1 ]; then
    # What a Plugin's package or an account installed belongs to them, and is removed by them.
    [ -z "$(ls -A /etc/yoke/plugins.d /usr/lib/yoke/plugins /usr/lib/yoke/interfaces 2>/dev/null | grep -v ':$' | grep -v '^$')" ] \
      || refuse "Plugins or interfaces are still installed under /etc/yoke/plugins.d or /usr/lib/yoke; remove the packages that put them there first"
    [ -z "$(ls -A /var/lib/yoke/apps 2>/dev/null)" ] \
      || refuse "/var/lib/yoke/apps still holds applications; each account removes its own first (yoke app remove <id>)"
  fi
  if has systemctl && [ -d /run/systemd/system ]; then systemctl disable yoke.service >/dev/null 2>&1 || true; fi
  for program in yoke yoke-core yoke-guardian yoke-auth yoke-compose; do
    [ ! -e "/usr/bin/$program" ] || { rm -f "/usr/bin/$program"; say "removed /usr/bin/$program"; }
  done
  for input in /usr/lib/systemd/system/yoke.service /usr/lib/systemd/user/yoke-guardian.service \
               /usr/lib/sysusers.d/yoke.conf /usr/lib/tmpfiles.d/yoke.conf; do
    [ ! -e "$input" ] || { rm -f "$input"; say "removed $input"; }
  done
  if [ "$purge" -eq 1 ]; then
    for kept in /etc/yoke /var/lib/yoke /usr/lib/yoke; do
      [ ! -e "$kept" ] || { rm -rf "$kept"; say "removed $kept"; }
    done
    ! getent group yoke-apps >/dev/null 2>&1 || { groupdel yoke-apps && say "removed the group yoke-apps"; }
  fi
  if has systemctl && [ -d /run/systemd/system ]; then systemctl daemon-reload; fi
  if [ "$purge" -eq 1 ]; then
    say "Yoke is removed from this host, with its data."
  else
    say "Yoke is removed from this host; /etc/yoke, /var/lib/yoke and the group yoke-apps are kept (--purge removes them)."
  fi
  exit 0
fi

# The machine.
[ "$(uname -s)" = Linux ] || refuse "Yoke runs on Linux; this is $(uname -s)"
if [ -z "$arch" ]; then
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) refuse "no release is built for $(uname -m); Yoke is released for amd64 and arm64" ;;
  esac
fi
case "$arch" in
  amd64|arm64) ;;
  *) refuse "no release is built for $arch; Yoke is released for amd64 and arm64" ;;
esac

# The tools.
missing=""
for tool in tar gzip mktemp cmp grep sed cut; do has "$tool" || missing="$missing $tool"; done
if has sha256sum; then sum() { sha256sum "$1" | cut -d' ' -f1; }
elif has shasum; then sum() { shasum -a 256 "$1" | cut -d' ' -f1; }
else missing="$missing sha256sum"; fi
if has curl; then
  fetch() {
    case "$1" in file://*) proto="=file" ;; *) proto="=https" ;; esac
    curl --proto "$proto" --tlsv1.2 -fsSL -o "$2" "$1"
  }
elif has wget; then
  fetch() { wget -q --https-only -O "$2" "$1"; }
else missing="$missing curl-or-wget"; fi
[ -z "$missing" ] || refuse "missing:$missing — install them and run this again"

# The release.
if [ -z "$version" ]; then
  [ -n "$YOKE_RELEASE" ] || refuse "this copy of the script was not published with a release; name one with --version"
  version="$YOKE_RELEASE"
fi
case "$version" in
  *[!0-9.]*|""|.*|*.) refuse "$version is not a release number such as 0.12.0" ;;
esac

files="yoke-$version-linux-$arch.tar.gz"
[ "$runtime_only" -eq 1 ] || files="$files yoke-conformance-$version-linux-$arch.tar.gz"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT INT TERM

# The digests: the ones this copy carries for its own release, the manifest's for any other.
if [ "$version" = "$YOKE_RELEASE" ]; then
  say "checking against the digests published with $version"
  printf '%s\n' "$YOKE_DIGESTS" > "$work/digests"
else
  say "checking against the release manifest"
  fetch "$manifest" "$work/manifest.jsonl" || refuse "the release manifest cannot be read from $manifest"
  : > "$work/digests"
  for file in $files; do
    case "$file" in
      yoke-conformance-*) published="yoke-conformance-linux-$arch" ;;
      *) published="yoke-linux-$arch" ;;
    esac
    # A manifest line has a fixed shape and order: published, version, commit, digests, ...
    digest="$(grep -F "\"published\":\"$published\",\"version\":\"v$version\"," "$work/manifest.jsonl" \
      | sed -n 's/.*"digests":\["sha256:\([0-9a-f]\{64\}\)".*/\1/p' | head -n 1)"
    [ -n "$digest" ] || refuse "the release manifest lists no $published for $version — check the number; a release's lines are added just after it is published"
    printf '%s  %s\n' "$digest" "$file" >> "$work/digests"
  done
fi

# A file already kept is used as it is when it is the one released, and never replaced by a download
# without --force when it is not; both are settled before anything is downloaded.
expected() { grep -F "  $1" "$work/digests" | cut -d' ' -f1 | head -n 1; }
for file in $files; do
  [ -n "$(expected "$file")" ] || refuse "no digest is published for $file"
  if [ -e "$dir/$file" ] && [ "$(sum "$dir/$file")" != "$(expected "$file")" ] && [ "$force" -eq 0 ]; then
    refuse "$dir/$file is there and differs; --force replaces it"
  fi
done

# Every other file is downloaded beside the others first, and checked before anything is kept.
fetched=""
for file in $files; do
  if [ -e "$dir/$file" ] && [ "$(sum "$dir/$file")" = "$(expected "$file")" ]; then
    say "using $dir/$file, already kept and checked"
    continue
  fi
  say "downloading $file"
  fetch "$releases/v$version/$file" "$work/$file" || refuse "$file cannot be downloaded from $releases/v$version/ — is $version a release?"
  found="$(sum "$work/$file")"
  [ "$found" = "$(expected "$file")" ] || refuse "$file is not the file released: its digest is $found, the published one $(expected "$file"); nothing was kept"
  fetched="$fetched $file"
done

mkdir -p "$dir"
for file in $fetched; do
  cp "$work/$file" "$dir/$file"
  say "kept $dir/$file, checked"
done

host_file="$dir/yoke-$version-linux-$arch.tar.gz"
tools_file="$dir/yoke-conformance-$version-linux-$arch.tar.gz"
[ "$runtime_only" -eq 0 ] || tools_file=""

# place copies an unpacked file into a directory, refusing to replace a different one unless upgrading or
# forced.
place() { # <file> <target> <mode>
  if [ -e "$2" ] && ! cmp -s "$1" "$2" && [ "$force" -eq 0 ] && [ "$upgrade" -eq 0 ]; then
    refuse "$2 is there and differs, from an earlier installation; --upgrade replaces it"
  fi
  mkdir -p "$(dirname "$2")"
  cp "$1" "$2.yoke-install" && chmod "$3" "$2.yoke-install" && mv -f "$2.yoke-install" "$2"
}

unpacked="$work/unpacked"
mkdir -p "$unpacked"
if [ -n "$install" ]; then
  tar -xzf "$host_file" -C "$unpacked"
  [ -z "$tools_file" ] || tar -xzf "$tools_file" -C "$unpacked" yoke-compose
fi

if [ "$install" = account ]; then
  programs="yoke yoke-core yoke-guardian yoke-auth"
  [ -z "$tools_file" ] || programs="$programs yoke-compose"
  for program in $programs; do place "$unpacked/$program" "$bin/$program" 0755; done
  say "installed $programs in $bin"

  # The install root: one of this account's own, with the properties the manager checks.
  config="${XDG_CONFIG_HOME:-$HOME/.config}/yoke/manager.conf"
  if [ -e "$config" ]; then
    say "kept $config as it is"
  else
    root="$HOME/yoke/apps"
    mkdir -p "$root" && chmod 2775 "$root"
    mkdir -p "$(dirname "$config")"
    printf 'install_root: %s\n' "$root" > "$config"
    say "made the install root $root, named in $config"
  fi

  # The guardian's user unit, naming where the program is.
  units="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  sed "s|/usr/bin/yoke-guardian|$bin/yoke-guardian|" "$unpacked/packaging/yoke-guardian.service" > "$work/yoke-guardian.service"
  place "$work/yoke-guardian.service" "$units/yoke-guardian.service" 0644
  say "installed the guardian's user unit in $units"
  if [ "$at_boot" -eq 1 ]; then
    if has systemctl && systemctl --user daemon-reload 2>/dev/null; then
      systemctl --user enable yoke-guardian.service
      if has loginctl && loginctl enable-linger "$(id -un)"; then
        say "this account's instances come back when the host boots"
      else
        warn "this account's services cannot be made to run without a login; ask an administrator for 'loginctl enable-linger $(id -un)'"
      fi
    else
      warn "this account has no systemd user manager answering; instances will not come back at boot"
    fi
  fi
  case ":$PATH:" in
    *":$bin:"*) ;;
    *) warn "$bin is not on PATH; add it, for instance: export PATH=\"$bin:\$PATH\"" ;;
  esac
fi

if [ "$install" = host ]; then
  programs="yoke yoke-core yoke-guardian yoke-auth"
  [ -z "$tools_file" ] || programs="$programs yoke-compose"
  for program in $programs; do place "$unpacked/$program" "/usr/bin/$program" 0755; done
  place "$unpacked/packaging/yoke.sysusers" /usr/lib/sysusers.d/yoke.conf 0644
  place "$unpacked/packaging/yoke.tmpfiles" /usr/lib/tmpfiles.d/yoke.conf 0644
  place "$unpacked/packaging/yoke.service" /usr/lib/systemd/system/yoke.service 0644
  place "$unpacked/packaging/yoke-guardian.service" /usr/lib/systemd/user/yoke-guardian.service 0644
  if [ -e /etc/yoke/core.yaml ]; then
    say "kept /etc/yoke/core.yaml as it is"
  else
    place "$unpacked/packaging/core.yaml" /etc/yoke/core.yaml 0644
  fi
  systemd-sysusers yoke.conf
  systemd-tmpfiles --create yoke.conf
  if [ -d /run/systemd/system ]; then systemctl daemon-reload; fi
  say "installed $programs in /usr/bin, and the packaging inputs"
  say "an account installs applications once it is in the group yoke-apps: usermod -aG yoke-apps <account>"
fi

# What the host lacks and Yoke would use.
if [ -n "$install" ]; then
  pam=0
  for candidate in /lib/libpam.so.0 /lib64/libpam.so.0 /usr/lib/libpam.so.0 /usr/lib64/libpam.so.0 \
                   /lib/*/libpam.so.0 /usr/lib/*/libpam.so.0; do
    [ -e "$candidate" ] && pam=1
  done
  if [ "$pam" -eq 0 ]; then
    warn "no PAM library was found; yoke-auth will not run, and a channel that signs its callers in will refuse them"
  fi
  [ -d /run/systemd/system ] || warn "this host was not booted with systemd; nothing will start Yoke when it boots"
fi

say ""
say "Yoke $version for $arch is in $dir."
case "$install" in
  account)
    if [ "$upgrade" -eq 1 ]; then say "Upgraded for this account. Start again each instance you stopped: yoke instance start <name>"
    else say "Installed for this account. Check it: yoke app list && yoke instance list && echo installed"; fi ;;
  host)
    if [ "$upgrade" -eq 1 ]; then say "Upgraded for the whole host. Start the service form again with: yoke service start"
    else say "Installed for the whole host. Check the service form with: yoke service check"; fi ;;
  *) say "Install it: run this again with --install account, or with sudo and --install host." ;;
esac
