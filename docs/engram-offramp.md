# Engram Offramp Runbook

> Operational runbook for migrating off Engram as the memory backend and
> using `nt-cli` as the sole local memory surface. Read this start to finish
> before advancing between phases.

**Current phase**: shadow

## At a glance

| You want to... | Go to |
|----------------|-------|
| Understand the rollout phases | [Phases](#phases) |
| Check if you can advance to the next phase | [Readiness gates (G1–G6)](#readiness-gates-g1g6) |
| Roll back right now | [Rollback runbook](#rollback-runbook) |
| Turn on debug logs for an MCP failure | [Observability: `NTCLI_MCP_DEBUG`](#observability-ntcli_mcp_debug) |
| Confirm the offramp is done | [Exit criteria checklist](#exit-criteria-checklist) |

## Phases

The offramp ships in three phases. **Do not skip a phase.** Each phase has
preconditions in the readiness gate table below.

### 1. Shadow

Both Engram memory tools and `nt-cli` memory tools are registered on the MCP
host. Agents keep their existing habits; `nt-cli` is exercised in parallel.
No behavior change for end users.

- Goal: collect parity evidence and surface gaps without risk.
- Exit: G1, G2, G3, G5, G6 all green → advance to **Partial**.

### 2. Partial cutover (pilot)

A pilot host profile disables Engram memory tools. `nt-cli` is the only memory
surface for pilot agents during the soak window. Engram remains one config
toggle away as an escape hatch.

- Goal: prove the system works without Engram for a sustained window.
- Soak window: minimum **7 calendar days** with zero rollback triggers.
- Exit: G3, G4 green → advance to **Full**.

### 3. Full cutover (default)

The default host profile ships with Engram memory tools off. `nt-cli` is the
canonical backend. Engram is re-enabled only via an explicit, documented
opt-in flag.

- Goal: make `nt-cli` the default and Engram an opt-in.

## Readiness gates (G1–G6)

Each gate is a hard precondition. Do not advance phase if any required gate
for that transition is not green.

| Gate | Required for | Acceptance criterion | How to verify |
|------|--------------|----------------------|---------------|
| G1 Tool parity | Shadow → Partial | `parity_test.go` green; required-tool-set assertion passes 100% | `go test ./internal/mcp/...` |
| G2 Operation parity | Shadow → Partial | save/recall/list/get/update/delete succeed on CLI + MCP for sample N≥10 | Manual checklist below |
| G3 Backup verified | Any → Next | `~/.nt-cli/data.db` snapshot exists and restore tested once | `cp` snapshot + dry-run restore |
| G4 Soak clean | Partial → Full | Pilot soak window completes with zero rollback triggers fired | Operator log review |
| G5 Docs published | Shadow → Partial | README "Engram Offramp" + this runbook merged | `git log` |
| G6 Observability ready | Shadow → Partial | `NTCLI_MCP_DEBUG` path documented and produces logs on failure | See [`NTCLI_MCP_DEBUG`](#observability-ntcli_mcp_debug) |

If any required gate is not satisfied, **hold at the current phase** and name
the failing gate in your hold notice (e.g. "Holding at shadow: G1 failing —
parity_test.go reports missing tool `local_get`").

### G2 operation parity sample (N≥10)

Run each operation at least once on both the CLI and the MCP surface, mixing
inputs across the sample. Record pass/fail per row.

| # | Operation | CLI command | MCP tool | Pass? |
|---|-----------|-------------|----------|-------|
| 1 | save | `nt-cli save "..."` | `local_save` | |
| 2 | recall | `nt-cli recall "..."` | `local_recall` | |
| 3 | list | `nt-cli list 20` | `local_list` | |
| 4 | get | `nt-cli get <id>` | `local_get` | |
| 5 | update | `nt-cli update <id> "..."` | `local_update` | |
| 6 | delete | `nt-cli delete <id>` | `local_delete` | |
| 7–10 | repeat any of the above with edge inputs (empty query, missing id, large content) | | | |

## Host profile toggle (`NTCLI_PROFILE`)

The wrapper at `scripts/opencode-mcp-dev.sh` reads a single env var,
`NTCLI_PROFILE`, that names the active host profile. The wrapper does **not**
itself enable or disable Engram — the actual on/off is governed by the
OpenCode MCP host config (which MCP servers it registers). The profile is
the operator-visible label that records *which* configuration is in effect
and makes it verifiable from tests and logs.

| Profile | Engram memory tools | nt-cli memory tools | Use during |
|---------|---------------------|---------------------|------------|
| `shadow` (default) | registered | registered | Shadow phase |
| `pilot` | not registered | registered | Partial cutover |

### Toggling at the host

The toggle is a **single config change** in `~/.config/opencode/opencode.json`.

```jsonc
{
  "mcp": {
    // Default / shadow profile: keep Engram registered alongside nt-cli.
    "engram": {
      "type": "local",
      "command": ["/path/to/engram", "mcp"]
    },
    "ntcli": {
      "type": "local",
      "command": ["/opt/nt-cli/scripts/opencode-mcp-dev.sh"],
      "environment": { "NTCLI_PROFILE": "shadow" }
    }

    // Pilot profile (Engram-off): comment out the "engram" entry above
    // AND set NTCLI_PROFILE=pilot below. nt-cli becomes the sole memory
    // surface. Reconnect OpenCode for the change to take effect.
    //
    // "ntcli": {
    //   "type": "local",
    //   "command": ["/opt/nt-cli/scripts/opencode-mcp-dev.sh"],
    //   "environment": { "NTCLI_PROFILE": "pilot" }
    // }
  }
}
```

### Verifying the active profile

```bash
# Print resolved profile without launching MCP:
NTCLI_PROFILE=pilot /opt/nt-cli/scripts/opencode-mcp-dev.sh --print-profile
# → pilot

# On every real launch the wrapper emits a marker on stderr:
# ntcli-mcp-dev profile=pilot
```

Unknown values (e.g. typos like `pilto`) cause the wrapper to exit with a
non-zero status and a stderr message that names both the offending value
and the valid set. Operators cannot silently end up in an undefined
profile.

### Rollback via the toggle

A rollback from pilot to shadow is one config change: re-add the `engram`
entry and set `NTCLI_PROFILE=shadow` (or remove the env var). Reconnect
OpenCode. No nt-cli or Engram data is touched.

## Observability: `NTCLI_MCP_DEBUG`

When an MCP call fails or behaves unexpectedly, enable structured debug logs
to identify the failing tool.

### Activation

```bash
# One-off run
NTCLI_MCP_DEBUG=1 /opt/nt-cli/scripts/opencode-mcp-dev.sh

# Persistent, for an OpenCode session: add the env var to the MCP host config
# entry that launches the wrapper, then reconnect OpenCode.
```

### Example log output

When enabled, the MCP server emits structured lines on stderr identifying the
tool, request id, and the failure reason:

```
ntcli-mcp tool=local_get id=42 status=error reason="not found"
ntcli-mcp tool=local_save id=43 status=ok bytes=128
ntcli-mcp tool=local_recall id=44 status=error reason="empty query"
```

If `NTCLI_MCP_DEBUG` is unset or empty, the server stays quiet on stderr and
behaves identically to the default profile.

## Rollback runbook

Execute this procedure end-to-end when any rollback trigger fires. The
procedure is **non-destructive to Engram** — no Engram data is modified or
deleted by `nt-cli` or by these steps.

### Triggers (any one of these → roll back)

- `internal/mcp/parity_test.go` fails after a previously-green run.
- A user reports data loss attributable to `nt-cli`.
- MCP tool registration error on host startup.
- Soak-window error rate above the documented threshold.

### Steps

1. **Restore the prior MCP host profile** (Engram memory tools enabled).
   - Revert the host config change to the previous Engram-on profile.
   - Reconnect / reload the MCP host so it relaunches the MCP process.
2. **Restore the DB snapshot only if corruption is suspected.**
   ```bash
   # Snapshots live next to the live DB; pick the most recent.
   ls -lt ~/.nt-cli/data.db.snapshot.*
   cp ~/.nt-cli/data.db.snapshot.<timestamp> ~/.nt-cli/data.db
   ```
3. **Re-run the parity test** to confirm restoration:
   ```bash
   go test ./internal/mcp/...
   ```
4. **Save a post-mortem note** via `nt-cli`:
   ```bash
   nt-cli save "Engram offramp rollback <date>: trigger=<...>; cause=<...>; action=<...>"
   ```

If parity is still red after step 3, stop and escalate before any further
phase work.

## Exit criteria checklist

The offramp is **done** when every box below is checked. This mirrors the
proposal's Definition of Done.

- [ ] Capability parity verified: save/recall/list/get/update/delete on CLI + MCP.
- [ ] `internal/mcp/parity_test.go` green, including the required-tool-set assertion.
- [ ] Reversible migration path documented (backup + rollback).
- [ ] Soak window completed with Engram tools disabled in a host profile.
- [ ] `NTCLI_MCP_DEBUG` observability path documented and produces logs on failure.
- [ ] README "Engram Offramp" section + this runbook published.
- [ ] Default host profile ships with Engram memory tools off; opt-in path documented.
- [ ] Rollback trigger list is discoverable from the README.
