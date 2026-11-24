#!/usr/bin/env bash
# Drive the Xvfb session: ux.sh {shot [name]|click X Y|dblclick X Y|type TEXT|key KEYS|move X Y|wait SECS|window|size|oauth-url}
set -euo pipefail
# shellcheck source=env.sh
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
cmd=${1:-}; shift || true
case "$cmd" in
  shot) mkdir -p "$UX/shots"; f="$UX/shots/${1:-$(date +%H%M%S)}.png"
        import -window root "$f"
        # keep Read-viewable: downscale anything wider than 2000px, original kept as NAME-full.png
        if (( $(identify -format %w "$f") > 2000 )); then cp "$f" "${f%.png}-full.png"; mogrify -resize 1470x "$f"; fi
        echo "$f" ;;
  click) xdotool mousemove "$1" "$2" click 1 ;;
  dblclick) xdotool mousemove "$1" "$2" click --repeat 2 --delay 80 1 ;;
  type) xdotool type --delay 20 -- "$1" ;;
  key) xdotool key -- "$@" ;;
  move) xdotool mousemove "$1" "$2" ;;
  wait) sleep "$1" ;;
  window) xdotool search --onlyvisible --name . getwindowgeometry %@ ;;
  size) echo "${UX_LOGICAL_W}x${UX_LOGICAL_H} logical (scale ${UX_SCALE}x, MacBook Air 13 M4), usable ${UX_LOGICAL_W}x${UX_USABLE_H}, Xvfb $(xdotool getdisplaygeometry)" ;;
  oauth-url) cat "$MAILRAT_DATA_DIR/oauth-url.txt" ;;
  *) sed -n 2p "$0"; exit 2 ;;
esac
