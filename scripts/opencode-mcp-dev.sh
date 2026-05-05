#!/usr/bin/env bash
set -euo pipefail

cd /opt/nt-cli
exec go run ./cmd/nt-cli mcp
