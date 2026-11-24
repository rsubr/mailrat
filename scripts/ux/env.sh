# shellcheck shell=bash
# Shared settings for the UX scripts; sourced, not executed.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
UX="$ROOT/.ux"
export DISPLAY=:99
export LIBGL_ALWAYS_SOFTWARE=1
export MAILRAT_TOKEN_STORE=file   # tokens live in the profile's SQLite db, never the keyring
export MAILRAT_OAUTH_HEADLESS=1
export MAILRAT_OAUTH_PORT="${MAILRAT_OAUTH_PORT:-53682}"
export MAILRAT_DATA_DIR="${MAILRAT_DATA_DIR:-$UX/profile}"
mkdir -p "$UX"
# Display target: MacBook Air 13" M4, default scaled mode (2560x1664 native, 2x) = 1470x956 logical points.
# Emulated 1:1 logically (Xvfb 1470x956, FYNE_SCALE=1): same layout as the Mac, lighter shots than 2940x1912.
# Usable height excludes the ~38pt macOS menu bar. Do not test other resolutions.
export UX_LOGICAL_W=1470 UX_LOGICAL_H=956 UX_SCALE=2
export UX_USABLE_H=918
export FYNE_SCALE=1
