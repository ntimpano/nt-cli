# SDD Runtime Strategy (Token-Minimum)

Objetivo: minimizar consumo de tokens manteniendo calidad y trazabilidad.

## 1) Modelo por fase (task-based routing)

- `sdd-init`: **haiku** (barato, setup repetible)
- `sdd-explore`: **haiku** (exploración inicial, bajo costo)
- `sdd-propose`: **sonnet** (decisiones de alcance)
- `sdd-spec`: **sonnet** (contrato funcional)
- `sdd-design`: **sonnet** (arquitectura y riesgos)
- `sdd-tasks`: **sonnet** (descomposición y forecasting)
- `sdd-apply`: **sonnet** (implementación + tests)
- `sdd-verify`: **sonnet** (gate de calidad)
- `sdd-archive`: **haiku** (consolidación)
- `sdd-onboard`: **haiku** (guía operativa)

## 2) Reglas anti-derroche

1. No copiar artefactos enteros entre fases; pasar references (`topic_key`, ruta).
2. Recuperar por query específica (ntcli-first), no búsquedas abiertas.
3. Ejecutar verify solo cuando apply reporta cambios reales.
4. Evitar re-runs completos: preferir fix-batches dirigidos por findings.
5. No usar modelo premium para formateo/documentación mecánica.

## 3) Estrategia de slices

- Límite cognitivo: PRs chicas y autónomas.
- Si forecast de tasks marca riesgo alto, encadenar PRs automáticamente.
- Cada slice debe traer su propia evidencia mínima.

## 4) Memoria

- Source of truth y único backend: `ntcli_local_*`.
- Engram NO existe en este stack. Nunca llamar `mem_*`.
