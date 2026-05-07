# Changelog

## v0.5.4 (unreleased)

### Security hardening
- MCP debug logging is now opt-in only via `NT_CLI_MCP_DEBUG=1` and writes to `~/.nt-cli/logs/mcp.log` with file mode `0600`.
- Runtime `config.json` is now persisted with file mode `0600` to reduce exposure of local credentials.

### Installer behavior
- `scripts/install.sh` now fails fast when `nt-cli init --non-interactive` fails, with actionable error guidance.
