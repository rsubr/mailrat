#!/usr/bin/env bash
set -euo pipefail

CGO_ENABLED=0 go build -ldflags="-s -w" -o mailrat ./cmd/mailrat
