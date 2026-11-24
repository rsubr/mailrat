#!/usr/bin/env bash
# Stop the app and restore the profile from .ux/golden (or wipe it if none).
set -euo pipefail
# shellcheck source=env.sh
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
"$(dirname "${BASH_SOURCE[0]}")/stop.sh"
rm -rf "$MAILRAT_DATA_DIR"
if [[ -d "$UX/golden" ]]; then cp -a "$UX/golden" "$MAILRAT_DATA_DIR"; echo "restored golden"
else echo "no golden; profile wiped"; fi
