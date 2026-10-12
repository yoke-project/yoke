#!/usr/bin/env bash
# Starts the account's own systemd on a runner, where the service form's unit is verified at L3: the
# account lingers, and the runtime directory systemd uses is named for the steps that follow.
set -euo pipefail
sudo loginctl enable-linger "$USER"
runtime="/run/user/$(id -u)"
echo "XDG_RUNTIME_DIR=$runtime" >> "$GITHUB_ENV"
export XDG_RUNTIME_DIR="$runtime"
timeout 60 sh -c 'until systemctl --user is-system-running 2>/dev/null | grep -qE "running|degraded"; do sleep 1; done'
