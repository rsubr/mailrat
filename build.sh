#!/usr/bin/env bash
# Build mailrat for the current OS. The Fyne UI needs cgo (OpenGL), so this
# must run natively: on macOS it needs Xcode command line tools; on Linux,
# gcc plus libgl1-mesa-dev and the X11 dev packages.
#
#   ./build.sh          → ./mailrat            (binary)
#   ./build.sh app      → ./mailrat.app        (macOS .app bundle via fyne)
set -euo pipefail

if [[ "${1:-}" == "app" ]]; then
  command -v fyne >/dev/null || go install fyne.io/tools/cmd/fyne@latest
  cd cmd/mailrat
  fyne package -os darwin -release
  rm -rf ../../mailrat.app && mv mailrat.app ../../mailrat.app
else
  CGO_ENABLED=1 go build -ldflags="-s -w" -o mailrat ./cmd/mailrat
fi
