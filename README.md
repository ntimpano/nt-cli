# nt-cli

Local-first CLI de memoria personal en Go + SQLite, con modo MCP.

## Comandos

```bash
go run ./cmd/nt-cli init
go run ./cmd/nt-cli save "idea importante"
go run ./cmd/nt-cli recall "idea"
go run ./cmd/nt-cli list 20
go run ./cmd/nt-cli delete 3
go run ./cmd/nt-cli mcp
```

Base local: `~/.nt-cli/data.db`

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
