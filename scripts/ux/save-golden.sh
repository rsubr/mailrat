#!/usr/bin/env bash
# Snapshot the e2e profile (after the human logged in) to .ux/golden.
set -euo pipefail
# shellcheck source=env.sh
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
"$(dirname "${BASH_SOURCE[0]}")/stop.sh"
rm -rf "$UX/golden"
cp -a "$MAILRAT_DATA_DIR" "$UX/golden"
echo "saved $UX/golden"
