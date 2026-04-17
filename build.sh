#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# build.sh — mailrat build script
# Usage: ./build.sh [options]
#   -o <path>   Output binary path (default: ./mailrat)
#   -v          Verbose output
#   --release   Strip debug info and disable CGO (smaller, static binary)
# ---------------------------------------------------------------------------

BINARY_NAME="mailrat"
OUTPUT="./mailrat"
VERBOSE=false
RELEASE=false
CMD_PKG="./cmd/cli"

usage() {
  grep '^#' "$0" | sed 's/^# \{0,1\}//' | tail -n +2
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)        OUTPUT="$2"; shift 2 ;;
    -v)        VERBOSE=true; shift ;;
    --release) RELEASE=true; shift ;;
    -h|--help) usage ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

log() { echo "[build] $*"; }
vlog() { $VERBOSE && echo "[build] $*" || true; }

# ---------------------------------------------------------------------------
# Go binary
# ---------------------------------------------------------------------------
build_go() {
  log "Building Go binary → $OUTPUT"

  local ldflags="-s -w"
  local gcflags=""

  if $RELEASE; then
    vlog "Release mode: stripping debug info"
    export CGO_ENABLED=0
  fi

  if $VERBOSE; then
    go build -v -ldflags "$ldflags" -o "$OUTPUT" "$CMD_PKG"
  else
    go build -ldflags "$ldflags" -o "$OUTPUT" "$CMD_PKG"
  fi

  log "Done → $OUTPUT"
}

# ---------------------------------------------------------------------------
# Frontend (Phase 3 placeholder)
# ---------------------------------------------------------------------------
build_frontend() {
  if [[ ! -d "frontend" ]]; then
    vlog "No frontend/ directory — skipping"
    return
  fi
  log "Building frontend..."
  # TODO: npm run build or wails build
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
build_go
build_frontend
