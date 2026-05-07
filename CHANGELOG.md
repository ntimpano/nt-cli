# Changelog

## v0.5.4 (unreleased)

### BREAKING
- `local_list` ahora filtra por proyecto activo por default cuando existe contexto de proyecto.

Migration note:
- Si necesitás el comportamiento global anterior (cross-project), usá `all_projects=true` en `local_list`.

### Security hardening
- MCP debug logging is now opt-in only via `NT_CLI_MCP_DEBUG=1` and writes to `~/.nt-cli/logs/mcp.log` with file mode `0600`.
- Runtime `config.json` is now persisted with file mode `0600` to reduce exposure of local credentials.

### Installer behavior
- `scripts/install.sh` now fails fast when `nt-cli init --non-interactive` fails, with actionable error guidance.

### Project scoping (cluster 2, PR2b)
- `Service.List`/`local_list` aplican scoping por proyecto activo por default y exponen bypass explícito con `all_projects=true`.
- `ImportRecords` ahora hace dedupe por `(project_id, topic_key, content_hash)` y estampa `project_id` activo en inserts para aislar imports entre proyectos.

### Store robustness (cluster 3, PR3)
- Restore ahora reabre SQLite reaplicando pragmas de integridad (`foreign_keys=ON`, `journal_mode=WAL`), preservando cascadas FK post-restore.
- `project_switch` (MCP y CLI) usa backups pre-switch únicos con patrón `pre-switch-<projectID>-<unix>.db`, deja de ocultar fallas de backup y aborta el switch cuando el backup falla.
- Se agrega retención keep-last-5 por proyecto para snapshots pre-switch, eliminando backups más viejos del mismo proyecto.
