# Changelog

## v0.5.4 (unreleased)

### BREAKING
- `local_list` ahora filtra por proyecto activo por default cuando existe contexto de proyecto.
- `local_list` ahora responde `{ "items": [...] }` con claves snake_case (`topic_key`, `created_at`, `updated_at`, `project_id`, etc.) en lugar del JSON crudo camelCase del struct.

Migration note:
- Si necesitás el comportamiento global anterior (cross-project), usá `all_projects=true` en `local_list`.
- Si consumís `local_list` desde fuera, actualizá el parser para leer `items` y claves snake_case.

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

### MCP handlers parity (cluster 4, PR4)
- `local_session_end` ahora aplica cierre estricto (`SessionEndStrict`): falla con `summary_required` si no existe `local_session_summary` previo para la sesión.
- `project_confirm` ahora mantiene la API pública y agrega create-if-new: si el candidato no existe, lo crea y lo deja activo en la misma operación.
- El schema del tool `relate` ahora publica `relation_type` con `enum` alineado al whitelist real (`related`, `supersedes`, `conflicts_with`, `refines`, `depends_on`).
- Se estandariza validación de argumentos MCP con `decodeArgs[T]` y respuesta JSON-RPC consistente `-32602 invalid arguments` en handlers que antes aceptaban unmarshal silencioso.
