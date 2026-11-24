#!/usr/bin/env bash
# Browser access to Xvfb :99: x11vnc (127.0.0.1:5900, no auth) + noVNC via websockify on the
# Tailscale IP (else 127.0.0.1) port 6080. Usage: vnc.sh start|stop|status|url. Idempotent.
set -euo pipefail
# shellcheck source=env.sh
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
WEB=/usr/share/novnc PORT=6080

bind_ip() { ip -4 -o addr show tailscale0 2>/dev/null | awk '{sub(/\/.*/,"",$4); print $4; exit}' || true; }
running() { [[ -f "$UX/$1.pid" ]] && kill -0 "$(cat "$UX/$1.pid")" 2>/dev/null; }
launch() { local n=$1; shift; running "$n" && return 0; nohup "$@" >"$UX/$n.log" 2>&1 & echo $! >"$UX/$n.pid"; }

case "${1:-status}" in
start)
  ip=$(bind_ip); ip=${ip:-127.0.0.1}
  launch x11vnc x11vnc -display :99 -forever -shared -localhost -rfbport 5900 -nopw -xkb -noxdamage
  launch websockify websockify --web "$WEB" "$ip:$PORT" 127.0.0.1:5900
  for _ in $(seq 50); do ss -ltn | grep -q "$ip:$PORT " && break; sleep 0.1; done
  "$0" url ;;
stop)
  for n in websockify x11vnc; do
    [[ -f "$UX/$n.pid" ]] && kill "$(cat "$UX/$n.pid")" 2>/dev/null
    rm -f "$UX/$n.pid"
  done ;;
status)
  for n in x11vnc websockify; do running "$n" && echo "$n: running" || echo "$n: stopped"; done ;;
url)
  ip=$(bind_ip)
  echo "http://${ip:-127.0.0.1}:$PORT/vnc.html?autoconnect=1&resize=scale&path=websockify"
  [[ -z $ip ]] && echo "no tailscale0: bound to 127.0.0.1; use: ssh -L 6080:127.0.0.1:6080 <user>@<host>"
  echo "no auth: relies on Tailscale" ;;
*) echo "usage: $0 start|stop|status|url" >&2; exit 2 ;;
esac
