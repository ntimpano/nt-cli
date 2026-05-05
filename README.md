# nt-cli

Local-first CLI de memoria personal en Go + SQLite, con modo MCP.

## Comandos

```bash
go run ./cmd/nt-cli init
go run ./cmd/nt-cli save "idea importante"
go run ./cmd/nt-cli recall "idea"
go run ./cmd/nt-cli list 20
go run ./cmd/nt-cli get 3
go run ./cmd/nt-cli update 3 "nuevo contenido"
go run ./cmd/nt-cli delete 3
go run ./cmd/nt-cli mcp
```

Base local: `~/.nt-cli/data.db`

`get <id>` imprime `id`, contenido y los timestamps `created_at` / `updated_at` en UTC.
`update <id> "..."` reemplaza el contenido y refresca `updated_at` (UTC); `created_at` no cambia.
Ambos comandos retornan exit code distinto de cero si el id no existe o es inválido.

## Herramientas MCP

| Tool | Descripción | Argumentos |
|------|-------------|------------|
| `local_save` | Guarda una nota local | `{ content: string }` |
| `local_recall` | Busca notas por texto | `{ query: string, limit?: integer }` |
| `local_list` | Lista notas recientes | `{ limit?: integer }` |
| `local_get` | Obtiene una nota por id | `{ id: integer }` |
| `local_update` | Actualiza el contenido de una nota por id | `{ id: integer, content: string }` |
| `local_delete` | Elimina una nota por id | `{ id: integer }` |

`local_get` y `local_update` reportan errores con `isError: true` cuando el id no existe, el id es inválido o el contenido es vacío/whitespace.

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

## Engram Offramp

`nt-cli` está reemplazando a Engram como backend de memoria. La migración se
ejecuta en tres fases (shadow → partial → full) con compuertas de readiness
medibles y un rollback reversible.

**Fase actual**: shadow

- Runbook completo y checklist de salida: [`docs/engram-offramp.md`](docs/engram-offramp.md)
- Compuertas de readiness G1–G6: ver [tabla en el runbook](docs/engram-offramp.md#readiness-gates-g1g6)

### Triggers de rollback

Si pasa cualquiera de estos, ejecutar el [rollback runbook](docs/engram-offramp.md#rollback-runbook):

- `internal/mcp/parity_test.go` falla luego de haber estado verde.
- Reporte de pérdida de datos atribuible a `nt-cli`.
- Error de registro de tools MCP al arrancar el host.
- Tasa de error en la ventana de soak por encima del umbral documentado.

El rollback es **no destructivo para Engram**: ningún paso modifica datos de Engram.
