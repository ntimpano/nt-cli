# MCP Contract Notes

Este documento resume contratos del servidor MCP de `nt-cli` que son relevantes para integraciones y tests de paridad.

## Contract parity (PR4 — MCP handlers)

### `local_session_end` (strict)

- El tool `local_session_end` usa cierre estricto (`SessionEndStrict`).
- Si la sesión no tiene una fila `summary` previa (vía `local_session_summary`), responde error de tool con texto que contiene `summary_required`.
- Si la sesión ya tiene summary, el cierre responde éxito normal.

Flujo recomendado:

1. `local_session_start`
2. `local_session_summary`
3. `local_session_end`

### `project_confirm` (create-if-new)

- `project_confirm` mantiene su shape pública: `{ "candidate": "<name>" }`.
- Si el candidato existe, cambia el proyecto activo a ese id.
- Si no existe, crea el proyecto con ese nombre y luego lo activa en la misma operación.

### `relate.relation_type` schema parity

- Cuando `NTCLI_FF_GRAPH=1`, el tool `relate` se anuncia en `tools/list`.
- El `inputSchema.properties.relation_type.enum` se alinea 1:1 con `app.AllowedRelationTypes`:
  - `related`
  - `supersedes`
  - `conflicts_with`
  - `refines`
  - `depends_on`

## Validación de argumentos MCP

`tools/call` usa decodificación tipada para validar args y devolver errores consistentes.

- Helper interno: `decodeArgs[T]`.
- Si los args no matchean el schema esperado (tipo inválido o JSON malformado), el servidor responde:
  - JSON-RPC code: `-32602`
  - message: `invalid arguments`

Esto evita aceptar zero-values por `json.Unmarshal` silencioso y garantiza comportamiento uniforme entre handlers.
