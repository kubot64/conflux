# Agent Instructions

This project uses **bd** (beads) for issue tracking. Run `bd onboard` to get started.

## TDD Policy

This project follows **Test-Driven Development**. Always follow the Red → Green → Refactor cycle.

**Rules:**

- Write a failing test first, then write the minimum production code to make it pass
- Each package has a paired `_test.go` file (e.g., `internal/config/config_test.go`)
- Use table-driven tests (standard Go convention)
- Run `go test ./...` before closing any issue — all tests must pass
- Do not write production code without a corresponding test

## Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work atomically
bd close <id>         # Complete work
bd sync               # Sync with git
```

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var

## Phase 1: Reference (Environment Variables, Exit Codes, Error Kinds, JSON Schema)

### Environment Variables

| Variable | Required | Description |
|---|---|---|
| `CONFLUENCE_URL` | Yes | Confluence base URL (e.g. `https://confluence.example.com`) |
| `CONFLUENCE_TOKEN` | Yes | Personal Access Token (PAT) |
| `CONFLUENCE_DEFAULT_SPACE` | No | Default space key used when `--space` is omitted |
| `CONFLUENCE_CLI_LOG` | No | Debug log file path. Omit to disable logging |
| `CONFLUENCE_CLI_TIMEOUT` | No | Command-wide timeout (e.g. `30s`, `2m`). Default: `30s`. Overrideable with `--timeout` flag |
| `CONFLUENCE_CLI_HOME` | No | Override `~/.confluence-cli/` directory |

### Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success (includes empty search results, `--if-exists skip`) |
| `1` | Validation error (invalid ID format, missing required flags) |
| `2` | Auth error (invalid PAT, insufficient permissions) |
| `3` | Network/server error (connection failure, 5xx, timeout, canceled) |
| `4` | Resource not found (404) |
| `5` | Conflict (`--if-exists error` with existing page, ambiguous title match) |

### Error Kinds (`kind` field in JSON error output)

| kind | exit code | description |
|---|---|---|
| `validation_error` | 1 | Input validation failure |
| `auth_error` | 2 | Authentication failure |
| `server_error` | 3 | Network failure or 5xx response |
| `timeout` | 3 | `context.DeadlineExceeded` — consider retry or increasing timeout |
| `canceled` | 3 | `context.Canceled` (SIGINT etc.) — intentional, do NOT retry |
| `not_found` | 4 | Resource not found |
| `conflict` | 5 | Conflict (duplicate title, etc.) |

### `--json` Output Schema

All `--json` output uses the unified envelope:

**Success (stdout)**
```json
{
  "schema_version": 1,
  "command": "<command name>",
  "result": { ... }
}
```

**Error (stderr)**
```json
{
  "schema_version": 1,
  "command": "<command name>",
  "error": {
    "code": 4,
    "kind": "not_found",
    "message": "page 12345 not found"
  }
}
```

**Warning (stderr)** — operation succeeded but a side effect (e.g. history write) failed:
```json
{
  "schema_version": 1,
  "command": "<command name>",
  "warning": {
    "kind": "history_write_failed",
    "message": "failed to write history: permission denied"
  }
}
```

#### `result` shape by command

- List/search commands (`space list`, `page search`, `page tree`, `alias list`, `history list`): `result` is an **array**
- `page get`: `result` is always an **array**; partial failures add `errors[]` at top level (exit 0)
- All other commands: `result` is an **object**

#### Phase 1 command examples

**`ping`**
```json
{"schema_version":1,"command":"ping","result":{"ok":true,"url":"https://confluence.example.com"}}
```

**`version`**
```json
{"schema_version":1,"command":"version","result":{"version":"0.1.0","commit":"abc1234","built_at":"2026-03-03"}}
```

## Phase 2: Command Reference

### `space list`

```bash
conflux space list [--json]
```

JSON `result`: array of `{key, name, url}`

```json
{"schema_version":1,"command":"space list","result":[
  {"key":"TEAM","name":"Team Space","url":"https://confluence.example.com/display/TEAM"}
]}
```

---

### `page search`

```bash
conflux page search [keyword] [--space SPACE] [--after YYYY-MM-DD] [--json]
```

- `keyword` — optional; searches full text
- `--space` — filter by space key; falls back to `CONFLUENCE_DEFAULT_SPACE`, then all spaces
- `--after` — filter pages modified after this date (ISO 8601)

JSON `result`: array of `{id, title, space, last_modified, url}`

```json
{"schema_version":1,"command":"page search","result":[
  {"id":"12345","title":"My Page","space":"TEAM","last_modified":"2024-01-01T00:00:00Z","url":"..."}
]}
```

---

### `page get`

```bash
conflux page get <page-ID> [page-ID ...] [--format markdown|html|storage] [--section NAME] [--max-chars N] [--json]
```

- Multiple IDs: partial failures do NOT abort; failed IDs appear in `errors[]` (exit 0)
- `--format`: `markdown` (default, storage→GFM), `html`/`storage` (raw storage XHTML)
- `--section`: extract content between named heading and next same-level heading
- `--max-chars`: truncate body to N Unicode codepoints

JSON `result`: always array of `{id, title, space, version, body, url}`
JSON `errors`: present only when some IDs failed; array of `{id, error}`

```json
{"schema_version":1,"command":"page get",
 "result":[{"id":"123","title":"Page A","space":"TEAM","version":5,"body":"# Hello\n\n...","url":"..."}],
 "errors":[{"id":"999","error":"resource not found"}]
}
```

---

### `page tree`

```bash
conflux page tree [--space SPACE] [--depth N] [--json]
```

- `--space` — required unless `CONFLUENCE_DEFAULT_SPACE` is set
- `--depth` — 1–10 (default 3)

JSON `result`: flat list ordered by depth; `parent_id` is `null` for root pages

```json
{"schema_version":1,"command":"page tree","result":[
  {"id":"100","title":"Root","parent_id":null,"depth":0,"url":"..."},
  {"id":"101","title":"Child","parent_id":"100","depth":1,"url":"..."}
]}
```

---

### `attachment list`

```bash
conflux attachment list <page-ID> [--json]
```

JSON `result`: array of `{id, filename, size, media_type, url}`

```json
{"schema_version":1,"command":"attachment list","result":[
  {"id":"att1","filename":"diagram.png","size":204800,"media_type":"image/png","url":"..."}
]}
```

---

### `alias` commands

Aliases map short names to page IDs or space keys, persisted in `$CONFLUENCE_CLI_HOME/alias.json`.

```bash
conflux alias set <name> <target> [--type page|space]  # default type: page
conflux alias get <name>
conflux alias list
conflux alias delete <name>
```

**`alias set` / `alias get`** JSON `result`: `{name, target, type}`

```json
{"schema_version":1,"command":"alias set","result":{"name":"home","target":"12345","type":"page"}}
```

**`alias list`** JSON `result`: array of `{name, target, type}`

```json
{"schema_version":1,"command":"alias list","result":[
  {"name":"home","target":"12345","type":"page"},
  {"name":"myspace","target":"MS","type":"space"}
]}
```

**`alias delete`** JSON `result`: `{deleted: "<name>"}`

```json
{"schema_version":1,"command":"alias delete","result":{"deleted":"home"}}
```

#### Using aliases with other commands

Aliases are resolved client-side — pass the resolved value to the command:

```bash
# Set alias
conflux alias set home 12345
# Use in page get
conflux page get $(conflux alias get home --json | jq -r '.result.target')
```

---

<!-- BEGIN BEADS INTEGRATION -->
## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Git-friendly: Auto-syncs to JSONL for version control
- Agent-optimized: JSON output, ready work detection, discovered-from links
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**

```bash
bd ready --json
```

**Create new issues:**

```bash
bd create "Issue title" --description="Detailed context" -t bug|feature|task -p 0-4 --json
bd create "Issue title" --description="What this issue is about" -p 1 --deps discovered-from:bd-123 --json
```

**Claim and update:**

```bash
bd update <id> --claim --json
bd update bd-42 --priority 1 --json
```

**Complete work:**

```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

1. **Check ready work**: `bd ready` shows unblocked issues
2. **Claim your task atomically**: `bd update <id> --claim`
3. **Work on it**: Implement, test, document
4. **Discover new work?** Create linked issue:
   - `bd create "Found bug" --description="Details about what was found" -p 1 --deps discovered-from:<parent-id>`
5. **Complete**: `bd close <id> --reason "Done"`

### Auto-Sync

bd automatically syncs with git:

- Exports to `.beads/issues.jsonl` after changes (5s debounce)
- Imports from JSONL when newer (e.g., after `git pull`)
- No manual export/import needed!

### Important Rules

- ✅ Use bd for ALL task tracking
- ✅ Always use `--json` flag for programmatic use
- ✅ Link discovered work with `discovered-from` dependencies
- ✅ Check `bd ready` before asking "what should I work on?"
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems

For more details, see README.md and docs/QUICKSTART.md.

<!-- END BEADS INTEGRATION -->

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
