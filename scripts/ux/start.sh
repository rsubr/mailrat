#!/usr/bin/env bash
# Start Xvfb :99, openbox, optional VNC/noVNC (UX_VNC=1, see vnc.sh) and mailrat. Idempotent.
set -euo pipefail
# shellcheck source=env.sh
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
mkdir -p "$MAILRAT_DATA_DIR"

running() { [[ -f "$UX/$1.pid" ]] && kill -0 "$(cat "$UX/$1.pid")" 2>/dev/null; }
launch() { # name cmd...
  local n=$1; shift
  running "$n" && return 0
  nohup "$@" >"$UX/$n.log" 2>&1 &
  echo $! >"$UX/$n.pid"
}

launch xvfb Xvfb :99 -screen 0 "${UX_LOGICAL_W}x${UX_LOGICAL_H}x24"
for _ in $(seq 50); do xdotool getdisplaygeometry >/dev/null 2>&1 && break; sleep 0.1; done
launch openbox openbox
[[ "${UX_VNC:-0}" == 1 ]] && "$(dirname "${BASH_SOURCE[0]}")/vnc.sh" start

# Reuse the dev machine's OAuth client if the profile has none.
if [[ ! -f "$MAILRAT_DATA_DIR/credentials.json" && -f "$HOME/.mailrat/credentials.json" ]]; then
  install -m 600 "$HOME/.mailrat/credentials.json" "$MAILRAT_DATA_DIR/credentials.json"
fi

fit() { # place the app window at 0,0 sized to the usable area
  local w
  for _ in $(seq 100); do
    w=$(xdotool search --onlyvisible --name . 2>/dev/null | head -1) && [[ -n $w ]] && break; sleep 0.2
  done
  [[ -n ${w:-} ]] && xdotool windowsize "$w" "$UX_LOGICAL_W" "$UX_USABLE_H" windowmove "$w" 0 0 || true
}
running mailrat && { fit; echo "already running"; exit 0; }
(cd "$ROOT" && go build -o "$UX/mailrat" ./cmd/mailrat)
launch mailrat "$UX/mailrat"
fit
echo "started; shots: scripts/ux/ux.sh shot"
