# nt-cli

Local-first CLI de memoria personal en Go + SQLite, con modo MCP.

## Comandos

```bash
go run ./cmd/nt-cli init
go run ./cmd/nt-cli save "idea importante"
go run ./cmd/nt-cli save --type=decision --topic-key=arch/auth --title="Auth Model" --scope=project "elegimos JWT"
go run ./cmd/nt-cli recall "idea"
go run ./cmd/nt-cli list 20
go run ./cmd/nt-cli get 3
go run ./cmd/nt-cli update 3 "nuevo contenido"
go run ./cmd/nt-cli delete 3
go run ./cmd/nt-cli session start sess-1
go run ./cmd/nt-cli session summary sess-1 "lo que hicimos"
go run ./cmd/nt-cli session end sess-1
go run ./cmd/nt-cli import ./obs.json
go run ./cmd/nt-cli import --dry-run ./obs.json
go run ./cmd/nt-cli backup /tmp/snap.db
go run ./cmd/nt-cli restore /tmp/snap.db
go run ./cmd/nt-cli doctor
go run ./cmd/nt-cli mcp
```

Base local: `~/.nt-cli/data.db`

`get <id>` imprime `id`, contenido y los timestamps `created_at` / `updated_at` en UTC.
`update <id> "..."` reemplaza el contenido y refresca `updated_at` (UTC); `created_at` no cambia.
Ambos comandos retornan exit code distinto de cero si el id no existe o es inválido.

### Metadata estructurada (M1)

`save` acepta flags opcionales para clasificar la nota:

| Flag | Default | Uso |
|------|---------|-----|
| `--title=...` | `""` | Título corto, buscable |
| `--type=...` | `manual` | Categoría: `decision`, `architecture`, `bugfix`, `pattern`, `config`, `discovery`, `learning`, `manual` |
| `--topic-key=...` | `""` | Clave estable para upsert (mismo `topic_key + scope` reemplaza la observación previa) |
| `--scope=...` | `project` | `project` o `personal` |

Si no pasás flags, `save` usa el path legacy y no toca la metadata. Si pasás cualquier flag, los defaults se aplican a los campos faltantes.

### Recall ranqueado (M2 — storage)

`recall` y `local_recall` ahora consultan una tabla virtual FTS5 (`memory_fts`) que se mantiene en sync con `memory_items` mediante triggers. Los resultados se ordenan por `bm25(memory_fts)` (mejor relevancia primero) en vez de por `created_at`.

- **Sin cambios para el caller**: la firma de `recall <query>` y `local_recall {query, limit}` no cambia.
- **Fallback transparente**: si FTS5 no está disponible o la tabla está corrupta, la consulta degrada a `LIKE` ordenado por `created_at DESC` sin reportar error al caller.
- **Migración aditiva**: al abrir una base previa M1, la migración crea `memory_fts` y reconstruye el índice (`INSERT INTO memory_fts(memory_fts) VALUES('rebuild')`) para que las filas legacy queden buscables sin re-guardar nada.
- **Latencia objetivo**: p95 < 50ms con 10k filas (medido en `TestRecall_P95Under50ms`).

### Sessions, Import, Backup y Doctor (M3)

`nt-cli` ahora cubre el ciclo completo de operación local:

- **`session start|summary|end <id> [text]`**: registra el ciclo de vida de una sesión en la tabla `sessions` (schema v3). `start` falla si el id ya existe; `summary` y `end` requieren id existente. La tabla queda como log puro: no muta `memory_items`.
- **`import [--dry-run] <file.json>`**: importa observaciones desde un JSON al store local. Idempotente: deduplica por `(topic_key, sha256(content))` mediante el índice único `memory_items_dedupe`. `--dry-run` muestra cuántas filas se insertarían sin tocar la base. Defaults: `type=manual`, `scope=project`. **No usa ni depende de ningún backend externo**.
- **`backup <path>`**: snapshot atómico vía `VACUUM INTO` — un único archivo `.db` portable, schema-aware, restaurable en otra máquina sin pérdida.
- **`restore <path>`**: reemplaza la base activa por el snapshot. Hace una copia lateral `.restore-bak` antes de sobrescribir; si falla a mitad, restaura el original. Requiere reabrir el proceso (`Init` es forward-only e idempotente).
- **`doctor`**: diagnóstico read-only. Imprime una línea con `schema_version`, salud de FTS5, `integrity_check` y row counts de `memory_items` y `sessions`. Mensajes de integridad no-ok se listan abajo del summary.

## Herramientas MCP

| Tool | Descripción | Argumentos |
|------|-------------|------------|
| `local_save` | Guarda una nota local | `{ content: string, title?, type?, topic_key?, scope? }` |
| `local_recall` | Busca notas por texto | `{ query: string, limit?: integer }` |
| `local_list` | Lista notas recientes | `{ limit?: integer }` |
| `local_get` | Obtiene una nota por id | `{ id: integer }` |
| `local_update` | Actualiza el contenido de una nota por id | `{ id: integer, content: string }` |
| `local_delete` | Elimina una nota por id | `{ id: integer }` |
| `local_session_start` | Inicia una sesión local | `{ id: string }` |
| `local_session_summary` | Adjunta resumen a la sesión | `{ id: string, summary: string }` |
| `local_session_end` | Cierra la sesión local | `{ id: string }` |
| `local_import` | Importa observaciones desde JSON (idempotente) | `{ path: string, dry_run?: boolean }` |
| `local_backup` | Snapshot portable del store local | `{ path: string }` |
| `local_restore` | Restaura el store local desde un snapshot | `{ path: string }` |
| `local_doctor` | Diagnóstico read-only del store local | `{}` |

`local_get` y `local_update` reportan errores con `isError: true` cuando el id no existe, el id es inválido o el contenido es vacío/whitespace.

Todos los tools `local_*` son **local-only**: operan exclusivamente sobre `~/.nt-cli/data.db` y no comunican con ningún backend externo.

## Integración con OpenCode (MCP)

Agregar en `~/.config/opencode/opencode.json`:

```json
{
  "mcp": {
    "ntcli": {
      "type": "local",
      "command": ["/opt/nt-cli/scripts/opencode-mcp-dev.sh"]
    }
  }
}
```

Antes, compilar binario:

```bash
cd /opt/nt-cli
go build -o nt-cli ./cmd/nt-cli
```

### Iteración sin rebuild/restart constantes

El wrapper `scripts/opencode-mcp-dev.sh` ejecuta `go run ./cmd/nt-cli mcp`.
Así, OpenCode siempre levanta el código más reciente al iniciar el MCP.

> Nota: igual necesitás reabrir/reconectar OpenCode para que relance el proceso MCP, pero no hace falta compilar manualmente cada cambio.

## Rollout

`nt-cli` se despliega en tres fases (shadow → partial → full) con
compuertas de readiness medibles y un rollback reversible vía host
profile.

**Fase actual**: shadow

- Runbook completo y checklist de salida: [`docs/rollout-runbook.md`](docs/rollout-runbook.md)
- Compuertas de readiness G1–G6: ver [tabla en el runbook](docs/rollout-runbook.md#readiness-gates-g1g6)
- Toggle de host (`NTCLI_PROFILE=shadow|pilot`): ver [Host profile toggle](docs/rollout-runbook.md#host-profile-toggle-ntcli_profile)

### Triggers de rollback

Si pasa cualquiera de estos, ejecutar el [rollback runbook](docs/rollout-runbook.md#rollback-runbook):

- `internal/mcp/parity_test.go` falla luego de haber estado verde.
- Reporte de pérdida de datos atribuible a `nt-cli`.
- Error de registro de tools MCP al arrancar el host.
- Tasa de error en la ventana de soak por encima del umbral documentado.

El rollback está **acotado a `nt-cli`**: no toca datos fuera de `~/.nt-cli/`
y la vuelta al perfil shadow es una sola edición de configuración.
