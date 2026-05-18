# nt-cli

[![Go Version](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)](./go.mod)
[![Build](https://github.com/ntimpano/nt-cli/actions/workflows/release.yml/badge.svg)](https://github.com/ntimpano/nt-cli/actions/workflows/release.yml)
[![License](https://img.shields.io/badge/license-unspecified-lightgrey)](#licencia)

**CLI de memoria personal local-first para agentes de IA**, con persistencia en SQLite y modo MCP para integrarlo con OpenCode.

<!-- Placeholder demo -->
<!-- ![demo](docs/demo.gif) -->

---

## ¿Qué es nt-cli?

`nt-cli` te deja guardar, buscar y recuperar contexto útil de forma persistente, **sin depender de servicios externos**.  
Está pensado para que vos (y tus agentes) no pierdan decisiones, aprendizajes ni estado entre sesiones.

## Features

- Local-first (todo queda en tu máquina)
- SQLite como base local (`~/.nt-cli/data.db`)
- Búsqueda full-text con FTS5
- Servidor MCP para OpenCode (`nt-cli mcp`)
- Manejo de sesiones (`start`, `summary`, `end`)
- Backup / restore atómico
- Scope por proyecto (contexto más limpio por repo)

## Quick Install

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/ntimpano/nt-cli/main/scripts/install.sh | bash
```

> Requiere OpenCode: https://opencode.ai

## Estado de rollout

- **Fase actual / Current phase**: **shadow**
- **Rollback triggers**: ver runbook en [`docs/rollout-runbook.md#rollback-runbook`](docs/rollout-runbook.md#rollback-runbook)

## Quick Start (30 segundos)

```bash
nt-cli init
nt-cli save "Definimos usar JWT para auth"
nt-cli recall "JWT"
nt-cli context --summary
nt-cli mcp
```

## Usage

### 1) Guardar notas con metadata

```bash
nt-cli save --title="Modelo de Auth" \
  --type=decision \
  --topic-key=arch/auth/jwt \
  --scope=project \
  "Elegimos JWT por simplicidad operativa"
```

- `--title`: título corto
- `--type`: tipo de nota (`decision`, `discovery`, `bug`, etc.)
- `--topic-key`: clave estable para agrupar/actualizar conocimiento
- `--scope`: `project` o `personal`

### 2) Buscar con recall

```bash
nt-cli recall "auth jwt"
nt-cli recall "migración sqlite"
```

Recupera lo más relevante por texto (FTS5) y te acelera volver al contexto real.

### 3) Sesiones (start → summary → end)

```bash
nt-cli session start sprint-23
nt-cli session summary sprint-23 "Cerramos auth + tests de integración"
nt-cli session end sprint-23
```

Útil para dejar trazabilidad de una sesión de trabajo sin mezclarlo con notas sueltas.

### 4) Operación local: backup / restore / doctor

```bash
nt-cli backup /tmp/ntcli-snapshot.db
nt-cli doctor
nt-cli restore /tmp/ntcli-snapshot.db
```

- `backup`: snapshot portable
- `doctor`: chequeo rápido de salud de la base
- `restore`: restaura estado desde snapshot

## OpenCode Integration (MCP)

Agregá esto en `~/.config/opencode/opencode.json`:

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

Compilá el binario antes:

```bash
cd /opt/nt-cli
go build -o nt-cli ./cmd/nt-cli
```

Para detalle de tools MCP y contratos, ver: **[`docs/mcp.md`](docs/mcp.md)**.

## Commands Reference

| Comando | Qué hace |
|---|---|
| `nt-cli init` | Inicializa configuración base |
| `nt-cli save "<texto>"` | Guarda una nota |
| `nt-cli recall "<query>"` | Busca notas por texto |
| `nt-cli list [limit]` | Lista notas recientes |
| `nt-cli get <id>` | Muestra una nota por ID |
| `nt-cli update <id> "<texto>"` | Actualiza una nota |
| `nt-cli delete <id>` | Elimina una nota |
| `nt-cli context --summary` | Resumen de contexto actual |
| `nt-cli session start <id>` | Inicia sesión |
| `nt-cli session summary <id> "<txt>"` | Guarda resumen de sesión |
| `nt-cli session end <id>` | Cierra sesión |
| `nt-cli import <archivo.json>` | Importa observaciones |
| `nt-cli backup <ruta.db>` | Crea snapshot local |
| `nt-cli restore <ruta.db>` | Restaura snapshot |
| `nt-cli doctor` | Diagnóstico de base local |
| `nt-cli mcp` | Ejecuta servidor MCP |

## Configuration

- **`AGENTS.md`**: define defaults de comportamiento de agentes para este repo.
- **`~/.nt-cli/profile.json`**: preferencias de usuario (idioma, tono, verbosidad, autoswitch de contexto, etc.).
- Ejemplo de perfil: `docs/profile.example.json`.

## Issues y soporte

¿Encontraste un bug o querés pedir una mejora?  
Abrí un issue acá: **https://github.com/ntimpano/nt-cli/issues**

## Contributing

Contribuciones bienvenidas: fixes, mejoras de DX, docs y tests.  
Si querés sumar algo, abrí un issue primero para alinear enfoque y evitar laburo duplicado: https://github.com/ntimpano/nt-cli/issues

## Licencia

Actualmente este repositorio **no declara una licencia explícita** en el root.  
Antes de reutilizar código fuera del proyecto, abrí un issue para confirmar condiciones de uso.
