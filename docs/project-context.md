# Project Context & Auto-Switch

_nt-cli v5 schema · PR3_

## Overview

nt-cli stores all memory in a single SQLite file. Starting with schema v5,
every memory row is associated with a **project** via a `project_id` foreign
key. A singleton `active_project` pointer determines which project is in scope
for reads and writes.

Project switching is **explicit** — nt-cli never silently changes context.
The OpenCode sidebar uses the MCP tools below to propose and confirm
context changes.

---

## MCP Tool Schema (sidebar contract)

All project tools are always-on (no feature flag). The server-info version
is `"0.1.0"`.

### `project_probe` — read-only detection

Inspects the given `cwd` (defaults to server cwd) and returns a candidate
without mutating state.

```jsonc
// Request
{
  "name": "project_probe",
  "arguments": { "cwd": "/path/to/repo" }
}

// Response (tool text, JSON)
{
  "status":     "known" | "new" | "none" | "ambiguous",
  "candidate":  "my-project",       // name of matched or proposed project
  "confidence": "high" | "low",
  "reason":     "fingerprint matched existing project",
  // Only present when status == "ambiguous":
  "candidates": [{ "id": 1, "name": "default", "root_path": "" }, ...]
}
```

**Side-effect rule**: `project_probe` NEVER mutates `active_project`.
Only `project_confirm` or `project_switch` may do so.

---

### `project_confirm` — commit a probed candidate

Accepts the `candidate` string returned by `project_probe` and sets it
as the active project. If the project is new it is created first.

```jsonc
// Request
{ "name": "project_confirm", "arguments": { "candidate": "my-project" } }

// Response
"confirmed project \"my-project\""
```

---

### `project_current` — read active project

Returns the current active project record.

```jsonc
// Request
{ "name": "project_current", "arguments": {} }

// Response (tool text, JSON)
{ "id": 1, "name": "default", "root_path": "" }
```

---

### `project_list` — enumerate all projects

Returns an array of all registered projects.

```jsonc
// Request
{ "name": "project_list", "arguments": {} }

// Response (tool text, JSON array)
[{ "id": 1, "name": "default", "root_path": "", "active": true }, ...]
```

---

### `project_switch` — switch active project by id

Takes a pre-switch backup automatically before switching. Returns the new
active project record.

```jsonc
// Request
{ "name": "project_switch", "arguments": { "id": 2 } }

// Response (tool text, JSON)
{ "id": 2, "name": "nt-cli", "root_path": "/opt/nt-cli" }
```

---

## MCP Resource: `nt-cli://project/active`

Advertised via `resources/list` when a project engine is available.
Allows the sidebar to passively observe the active project without a
tool call.

```jsonc
{
  "uri":         "nt-cli://project/active",
  "name":        "Active Project",
  "description": "The currently active nt-cli project context (id, name, root_path).",
  "mimeType":    "application/json"
}
```

Fetch the resource via `resources/read`:

```jsonc
// Request
{ "uri": "nt-cli://project/active" }

// Response (contents[0].text, JSON)
{ "id": 1, "name": "default", "root_path": "" }
```

---

## Recommended Sidebar Flow

```
1. On workspace open → call project_probe(cwd=<workspace root>)
2. If status == "known" && active matches → no action
3. If status == "known" but different project → ask user → call project_switch(id)
4. If status == "new" → ask user to name project → call project_confirm(candidate)
5. If status == "none" → no git repo; leave active project unchanged
6. If status == "ambiguous" → present candidates[] to user → call project_switch(id) on selection
```

---

## Auto-Backup Policy

(Implemented in `internal/app/scheduler.go`)

| Trigger | Backup path |
|---------|-------------|
| Pre-switch (MCP `project_switch`) | `~/.nt-cli/backups/pre-switch-mcp.db` |
| Pre-switch (CLI `project switch`)  | `~/.nt-cli/backups/pre-switch-cli.db` |
| Pre-restore                        | `~/.nt-cli/backups/pre-restore-<timestamp>.db` |
| Debounced ticker (Phase 4)         | `~/.nt-cli/backups/auto-<timestamp>.db` |

Retention policy (Phase 4): keep last **7 daily** + **4 weekly** backups;
older files are pruned automatically.

---

## Schema v5 Summary

```sql
CREATE TABLE projects (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  root_path   TEXT NOT NULL DEFAULT '',
  fingerprint TEXT NOT NULL DEFAULT '',
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE active_project (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE RESTRICT
);

-- memory_items gains a nullable project_id FK
ALTER TABLE memory_items ADD COLUMN project_id INTEGER REFERENCES projects(id);
```

Legacy rows (imported before v5) are backfilled to the `default` project
(id=1). No data is lost in the migration.
