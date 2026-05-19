#!/usr/bin/env bash
# opencode-mcp-dev.sh — launch the flint MCP server for OpenCode (dev mode).
#
# Host profile toggle (nt-cli rollout):
#   NTCLI_PROFILE=shadow  (default) Safe shadow profile used during the
#                         rollout's shadow phase.
#   NTCLI_PROFILE=pilot   Pilot host profile: nt-cli is the canonical
#                         memory backend on the host.
#
# This wrapper records the resolved profile in stderr so the active
# configuration is observable in logs and verifiable from tests. See
# docs/rollout-runbook.md for the full runbook.
#
# Flags / dev hooks:
#   --print-profile        Print the resolved profile to stdout and exit 0.
#                          Does NOT launch the MCP process. Used by the
#                          parity tests and by operators verifying the
#                          active profile.
#   NTCLI_PROFILE_DRYRUN=1 Skip exec'ing the MCP process after emitting
#                          the profile marker. Test-only escape hatch.

set -euo pipefail

valid_profiles="shadow pilot"

resolve_profile() {
  local raw="${NTCLI_PROFILE:-}"
  if [[ -z "$raw" ]]; then
    echo "shadow"
    return 0
  fi
  for p in $valid_profiles; do
    if [[ "$raw" == "$p" ]]; then
      echo "$p"
      return 0
    fi
  done
  echo "ntcli-mcp-dev: invalid NTCLI_PROFILE=\"$raw\"; valid profiles: $valid_profiles" >&2
  return 2
}

profile="$(resolve_profile)" || exit $?

if [[ "${1:-}" == "--print-profile" ]]; then
  echo "$profile"
  exit 0
fi

# Observability marker — matches the NTCLI_MCP_DEBUG style so operators
# can identify the active profile without enabling debug logging.
echo "ntcli-mcp-dev profile=$profile" >&2

if [[ "${NTCLI_PROFILE_DRYRUN:-}" == "1" ]]; then
  exit 0
fi

cd /opt/nt-cli
exec go run ./cmd/flint mcp
