# flint

[![Go Version](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)](./go.mod)
[![Build](https://github.com/ntimpano/nt-cli/actions/workflows/release.yml/badge.svg)](https://github.com/ntimpano/nt-cli/actions/workflows/release.yml)
[![License](https://img.shields.io/badge/license-unspecified-lightgrey)](#licencia)

**CLI de memoria personal local-first para agentes de IA**, con persistencia en SQLite y modo MCP para integrarlo con OpenCode.

<!-- Placeholder demo -->
<!-- ![demo](docs/demo.gif) -->

---

## ¿Qué es flint?

`flint` te deja guardar, buscar y recuperar contexto útil de forma persistente, **sin depender de servicios externos**.  
Está pensado para que vos (y tus agentes) no pierdan decisiones, aprendizajes ni estado entre sesiones.

## Features

- Local-first (todo queda en tu máquina)
- SQLite como base local (`~/.flint/flint.db`)
- Búsqueda full-text con FTS5
- Servidor MCP para OpenCode (`flint mcp`)
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
flint init
flint save "Definimos usar JWT para auth"
flint recall "JWT"
flint context --summary
flint mcp
```

## Usage

### 1) Guardar notas con metadata

```bash
flint save --title="Modelo de Auth" \
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
flint recall "auth jwt"
flint recall "migración sqlite"
```

Recupera lo más relevante por texto (FTS5) y te acelera volver al contexto real.

### 3) Sesiones (start → summary → end)

```bash
flint session start sprint-23
flint session summary sprint-23 "Cerramos auth + tests de integración"
flint session end sprint-23
```

Útil para dejar trazabilidad de una sesión de trabajo sin mezclarlo con notas sueltas.

### 4) Operación local: backup / restore / doctor

```bash
flint backup /tmp/flint-snapshot.db
flint doctor
flint restore /tmp/flint-snapshot.db
```

- `backup`: snapshot portable
- `doctor`: chequeo rápido de salud de la base
- `restore`: restaura estado desde snapshot

## OpenCode Integration (MCP)

Agregá esto en `~/.config/opencode/opencode.json`:

```json
{
  "mcp": {
    "flint": {
      "type": "local",
      "command": ["/opt/nt-cli/scripts/opencode-mcp-dev.sh"]
    }
  }
}
```

Compilá el binario antes:

```bash
cd /opt/nt-cli
go build -o flint ./cmd/flint
```

Para detalle de tools MCP y contratos, ver: **[`docs/mcp.md`](docs/mcp.md)**.

## Commands Reference

| Comando | Qué hace |
|---|---|
| `flint init` | Inicializa configuración base |
| `flint save "<texto>"` | Guarda una nota |
| `flint recall "<query>"` | Busca notas por texto |
| `flint list [limit]` | Lista notas recientes |
| `flint get <id>` | Muestra una nota por ID |
| `flint update <id> "<texto>"` | Actualiza una nota |
| `flint delete <id>` | Elimina una nota |
| `flint context --summary` | Resumen de contexto actual |
| `flint session start <id>` | Inicia sesión |
| `flint session summary <id> "<txt>"` | Guarda resumen de sesión |
| `flint session end <id>` | Cierra sesión |
| `flint import <archivo.json>` | Importa observaciones |
| `flint backup <ruta.db>` | Crea snapshot local |
| `flint restore <ruta.db>` | Restaura snapshot |
| `flint doctor` | Diagnóstico de base local |
| `flint mcp` | Ejecuta servidor MCP |

## Configuration

- **`AGENTS.md`**: define defaults de comportamiento de agentes para este repo.
- **`~/.flint/profile.json`**: preferencias de usuario (idioma, tono, verbosidad, autoswitch de contexto, etc.).
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
