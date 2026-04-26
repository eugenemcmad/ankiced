# Ankiced

`ankiced` is a local-first Anki maintenance app for safe operations on SQLite collections (`collection.anki2`).

It supports deck inspection, note editing, HTML cleanup, dry-run diffs, and transactional bulk updates with automatic backups.

Current version: `v0.1.1`.

## Product Direction

- CLI remains a basic admin interface for technical/power users.
- `v0.1.0` ships CLI, Web UI, and a Wails desktop shell.
- Web UI and desktop are the primary user-facing surfaces and reuse the same local API/HTML UI.
- Core Go business logic is shared across CLI/Web/Desktop.
- UI/UX target: modern, visually polished, low-friction user flows.
- UI includes a bottom real-time log console with detailed operation logs (`info`/`warn`/`error`), auto-scroll, and easy copy.
- Error handling model: user sees clear UX-friendly errors; log console keeps technical debug-ready details.
- Runtime shape now uses three apps: `ankiced` (CLI), `ankiced-web` (web shell that starts API and opens browser), and `ankiced-desktop` (Wails desktop shell).

## Web UI MVP / DoD

- `v0.1.0` Web/Desktop UI scope and acceptance criteria are defined in `tech-spec.md` (section `7`, subsections `7.A`/`7.B`/`7.C`).
- Scope includes screens, API contracts (`/api/v1`), real-time log streaming, and release readiness criteria.

## What You Can Do

- List decks and card counts.
- Search decks by substring in deck name.
- Rename decks with validation and conflict checks.
- List notes with pagination and filters (`deck`, `mod` range, text search).
- Find a note by numeric id (exact match).
- Search notes across the whole collection by text in `flds` (with pagination and optional `mod` range).
- Open note fields based on model metadata and edit in multiline mode.
- Preview cleaner output on one note before mass update.
- Run dry-run cleaner and inspect per-field diffs.
- Apply cleaner to a whole deck with explicit confirmation or `--force-apply`.
- Export dry-run report to JSON (`--report-file`).
- Use the default `html_cleaner` action template, backed by the shared action template registry.

Deck operations are based on the `decks` table (current Anki schema for new collections).
Note field metadata is resolved from modern notetype tables (`fields` by `ntid=mid`, ordered by `ord`), with compatibility fallback for legacy metadata layouts.
The HTML cleaner preserves only allowed basic tags (`b`, `i`, `br`, and safe `img` attributes) and strips other tags/attributes.

## Safety Model

- Backup is created before write operations.
- Bulk updates are written in a transaction.
- Worker pool is used for processing and single writer for DB writes.
- Fail-fast behavior on processing errors.
- Graceful shutdown (`SIGINT` / `SIGTERM`) with cleanup path.
- Long cleaner operations expose operation status and progress (`total`, `processed`, `changed`, `skipped`, `errors`, `stage`).

## Requirements

- Go `1.25+`

## Install / Build / Run

Run from project root:

```bash
go test ./...
go run ./cmd/ankiced --db-path "/path/to/collection.anki2"
go run ./cmd/ankiced-web --db-path "/path/to/collection.anki2" --http-addr "127.0.0.1:8080"
go run -tags production ./cmd/ankiced-desktop --db-path "/path/to/collection.anki2"
```

Build binary:

```bash
go build -o ./bin/ankiced ./cmd/ankiced
go build -o ./bin/ankiced-web ./cmd/ankiced-web
go build -tags production -o ./bin/ankiced-desktop ./cmd/ankiced-desktop
./bin/ankiced --db-path "/path/to/collection.anki2"
./bin/ankiced-web --db-path "/path/to/collection.anki2" --http-addr "127.0.0.1:8080"
./bin/ankiced-desktop --db-path "/path/to/collection.anki2"
```

For Wails frontend development diagnostics, you can still run the desktop app with `dev` tag:

```bash
go run -tags dev ./cmd/ankiced-desktop --db-path "/path/to/collection.anki2"
```

## Makefile Targets

```bash
make help
make test
make test-unit
make test-integration
make test-e2e
make run
make run-web
make run-desktop
make run-desktop-dev
make build
make build-web
make build-desktop
```

## Configuration

Priority (highest to lowest):

1. CLI flags
2. Environment variables
3. Config file (`yaml` or `json`)
4. OS default path discovery

### CLI Flags

- `--db-path` - path to `collection.anki2`
- `--anki-account` - Anki profile/account folder used for default DB lookup
- `--http-addr` - REST API bind address (default `127.0.0.1:8080`)
- `--config` - config file path
- `--backup-keep` - number of backups to keep
- `--workers` - worker count for batch processing
- `--force-apply` - allow cleaner apply without interactive confirmation
- `--verbose` - verbose output + debug error cause chain
- `--full-diff` - do not truncate diff output
- `--report-file` - write dry-run report as JSON
- `--page-size` - default pagination page size (default `10`)
- `--busy-timeout-ms` - SQLite `busy_timeout` in milliseconds (default `5000`)
- `--pragma-journal-mode` - SQLite `journal_mode` pragma, one of `WAL|DELETE|TRUNCATE|MEMORY|OFF|PERSIST` (default `WAL`)
- `--pragma-synchronous` - SQLite `synchronous` pragma, one of `OFF|NORMAL|FULL|EXTRA` (default `NORMAL`)

### Environment Variables

- `ANKICED_DB_PATH`
- `ANKICED_ANKI_ACCOUNT`
- `ANKICED_HTTP_ADDR`
- `ANKICED_BACKUP_KEEP`
- `ANKICED_WORKERS`
- `ANKICED_FORCE_APPLY`
- `ANKICED_VERBOSE`
- `ANKICED_PAGE_SIZE`
- `ANKICED_BUSY_TIMEOUT_MS`
- `ANKICED_PRAGMA_JOURNAL_MODE`
- `ANKICED_PRAGMA_SYNCHRONOUS`

### Config File Example

`config.example.yaml`:

```yaml
db_path: ""
anki_account: ""
http_addr: "127.0.0.1:8080"
backup_keep_last_n: 3
workers: 4
force_apply: false
verbose: false
full_diff: false
report_file: ""
default_page_size: 10
pragma_busy_timeout: 5000
pragma_journal_mode: "WAL"
pragma_synchronous: "NORMAL"
```

## REST API Mode

Start Web app (auto-start API + auto-open browser):

```bash
go run ./cmd/ankiced-web --http-addr "127.0.0.1:8080" --db-path "/path/to/collection.anki2"
```

Main endpoints (`/api/v1`):

- `GET /api/v1/decks`
- `GET /api/v1/decks/search?q=...`
- `PATCH /api/v1/decks/{id}`
- `GET /api/v1/notes?deck_id=...&limit=...&offset=...`
- `GET /api/v1/notes/{id}`
- `PATCH /api/v1/notes/{id}`
- `GET /api/v1/config`
- `POST /api/v1/config/save`
- `POST /api/v1/cleaner/preview` (`note_id`, optional `template_id`)
- `POST /api/v1/cleaner/dry-run` (`deck_id`, optional `template_id`)
- `POST /api/v1/cleaner/apply` (`deck_id`, `confirm=true`, optional `template_id`)
- `GET /api/v1/operations/{operation_id}`
- `GET /api/v1/logs/stream` (SSE)
- `GET /api/v1/logs?since_id=...` (SSE fallback / incremental polling)
- `POST /api/v1/app/exit`

API errors use a consistent JSON contract:

```json
{
  "message": "user-facing message",
  "recommended_action": "what the user should do next",
  "code": "MACHINE_READABLE_CODE",
  "details": "debug details",
  "correlation_id": "req-..."
}
```

Web shell extras:

- `Exit App` button triggers graceful local shutdown.
- `Connection & Settings` is shown near the bottom of the page above the log console.
- `Current Anki DB` shows the active DB file path.
- `Edit DB Path` reveals a DB path editor; changing it does not switch the current session automatically.
- `Show Current Config` displays effective runtime config.
- `Save Config` writes `ankiced.config.json` next to the running app binary.
- `Cleaner` is hidden by default and opens from the `Open Cleaner` button.
- Deck and note page sizes use `default_page_size` from config.

Cleaner dry-run/apply returns `202 Accepted` with `operation_id`; poll `GET /api/v1/operations/{operation_id}` for status/progress.
Realtime logs prefer SSE and fall back to incremental polling through `GET /api/v1/logs?since_id=...`, which is useful in the Wails desktop shell when streaming is unavailable.

## Interactive CLI Workflow

When app starts, you see:

- `1) List decks`
- `10) Search decks by text`
- `2) Rename deck`
- `3) List notes`
- `4) Edit note`
- `5) Preview cleaner`
- `6) Dry run cleaner`
- `7) Apply cleaner`
- `8) Find note by id`
- `9) Search notes (all decks)`
- `0) Exit`

### Typical Session

1. Choose `1` to inspect deck IDs.
2. Choose `3` to list notes in a deck:
   - enter `deck id`,
   - optional `limit` / `offset`,
   - optional `mod from` / `mod to`,
   - optional search text.
3. Choose `8` to load one note by id (no deck filter).
4. Choose `9` for collection-wide text search (required search substring); same pagination and `mod` filters as option `3`.
5. Choose `5` with one note ID to preview cleaner effect.
6. Choose `6` (dry-run) for deck-wide diff and summary.
7. Choose `7` to apply updates (with confirm unless `--force-apply`).

## Editing Notes (Option 4)

For each field:

- Current value is printed.
- You enter new multiline value.
- Finish input with a single line: `.end`

Escapes are supported in input (`\n`, `\t`, `\\`, etc.).

## Dry-Run and Bulk Apply

Dry-run output contains:

- field-level changes (`note`, `field`, `before`, `after`)
- summary (`processed`, `changed`, `skipped`, `errors`)

For long values:

- default output is truncated preview
- use `--full-diff` for full values

Optional report:

```bash
go run ./cmd/ankiced --db-path "/path/to/collection.anki2" --report-file "./dry-run-report.json"
```

The cleaner uses the default action template `html_cleaner`. API callers can pass `template_id`; omitting it uses the default template.

Account-based default lookup (without hardcoding DB path):

```bash
go run ./cmd/ankiced --anki-account "your-anki-profile-folder"
```

## Troubleshooting

- **`invalid decks metadata` / deck loading issues**
  - make sure you use a real Anki collection for your account/profile;
  - current implementation expects deck records in `decks` table (new schema).
- **Database path is empty**
  - set `--db-path` or `ANKICED_DB_PATH`.
  - or set `--anki-account` / `ANKICED_ANKI_ACCOUNT` for OS default lookup.
- **Permission denied**
  - check read/write access to DB and backup directory.
- **Deck/model not found**
  - list decks first, verify IDs.
- **Invalid escape sequence**
  - fix malformed escapes in multiline edit input.
- **Verbose diagnostics**
  - run with `--verbose` to include cause chain.

## Privacy Note

Do not commit personal Anki account names or emails to git.

Use one of these safe options:

- local environment variable: `ANKICED_ANKI_ACCOUNT`
- local, ignored config file (for example `config.yaml` / `config.yml`, already ignored in this project)

## Architecture

Project follows SOLID + Clean Architecture + DDD style boundaries:

- `internal/domain` - entities, rules, policies.
- `internal/application` - use-cases and ports.
- `internal/infrastructure` - SQLite/filesystem/config/render/sanitize adapters.
- `internal/interfaces/cli` - interactive terminal interface.
- `internal/interfaces/httpapi` - local Web UI and JSON/SSE API.
- `internal/bootstrap` - shared process bootstrap helpers for command entrypoints.
- `internal/presentation` - shared user-facing error formatting.
- `cmd/ankiced` - composition root and process bootstrap.
- `cmd/ankiced-web` - Web UI/API composition root and browser startup.
- `cmd/ankiced-desktop` - Wails desktop composition root.

## Tests

- Unit tests: `internal/**/**/*_test.go`
- Integration tests: `tests/integration`
- E2E tests: `tests/e2e` (CLI/API blackbox scope; browser UI E2E is not part of current MVP scope)

Run all:

```bash
go test ./...
```

