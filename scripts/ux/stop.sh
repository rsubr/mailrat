#!/usr/bin/env bash
# Stop everything start.sh launched.
# shellcheck source=env.sh
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
"$(dirname "${BASH_SOURCE[0]}")/vnc.sh" stop
for n in mailrat openbox xvfb; do
  [[ -f "$UX/$n.pid" ]] && kill "$(cat "$UX/$n.pid")" 2>/dev/null
  rm -f "$UX/$n.pid"
done
exit 0
