#!/usr/bin/env bash
set -euo pipefail
YAML="${1:-samples/shop-suite-with-issues.yaml}"
go run ./cmd/worker analyze "$YAML" out
