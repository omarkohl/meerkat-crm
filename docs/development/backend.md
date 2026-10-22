---
title: Backend Development
parent: Development
nav_order: 2
---

# Backend Development

## Setup

Requires current [Go version](https://go.dev/doc/install).

```sh
cd backend
cp .env.example .env   # then edit with your values
go mod tidy
```

## Running Locally

```sh
source .env
go run main.go
```

The server starts on `PORT` (default `8080`). Migrations run automatically.

## Adding a New Endpoint

Add a use case and an operation to the module the endpoint belongs to (or start a module, see
[Modules](#modules-hexagonal)): a typed input with `validate` tags and a `Resolve` method calling
`api.ValidateBody`, a handler in the module's `adapters/http`, and its registration. Then run
`make openapi`. A JSON route that is not a huma operation fails `routes/contract_test.go`; only
routes that send files, images or redirects are listed there as exceptions.

## Database Migrations

### Creating a Migration

```sh
cd backend && make migrate-create NAME=add_foo_column
```

This creates `NNNNNN_add_foo_column.up.sql` and `.down.sql` in `database/migrations/` (needs the
[`migrate` CLI](https://github.com/golang-migrate/migrate)); write a working `down`. Never edit an
existing migration ([ADR 0006](../adr/0006-never-edit-a-migration.md)); the round-trip test in
`database/` runs every pair up and down.

### Dropping the legacy thumbnail column

`contacts.photo_thumbnail` (base64 data URL) is kept only as the copy a downgrade needs.
`services.ExportLegacyThumbnails` moves it into files at startup, but it runs after the migrations.
A migration that drops the column (`DROP COLUMN photo_thumbnail`) must therefore never be the first
thing a database sees after the export-less version: ship it at least one release after the export,
and never edit that rule away. `RunMigrations` enforces it (`checkLegacyThumbnailsExported`): it
refuses to start when a pending migration drops the column while some contact still has a `data:`
thumbnail and no `photo_thumbnail_file`, and the error says to start the previous version once. Keep
the `DROP COLUMN photo_thumbnail` spelling so the check recognises the migration. The same release
removes `LegacyThumbnail`, `serveLegacyThumbnail` and the fallback in `ContactToVCard`.

### SQLite settings

The database is opened in WAL mode with `busy_timeout`, `synchronous=NORMAL`, foreign keys on and
`_txlock=immediate` (see `sqliteDSN` in `database/migrate.go`). Immediate transactions wait for the
write lock instead of failing with `SQLITE_BUSY` when they read before they write. Migrations run on
a separate connection without foreign key enforcement because they may recreate tables. Never copy
`meerkat.db` while the app runs; use the built-in [backups](../deployment.html#backups)
(`VACUUM INTO`).

### Search index (FTS5)

Search uses SQLite FTS5 tables `contacts_fts`, `notes_fts` and `activities_fts` (migrations 29 and
34). Each has the id of the indexed row as its rowid and is kept in sync by
`AFTER INSERT/UPDATE/DELETE` triggers on the real table; the tokenizer is
`unicode61 remove_diacritics 2`. The index is not scoped by user: queries always join back to the
real table and filter `user_id` and `deleted_at` there. `search.MatchQuery` quotes every typed word
as a prefix term, so user input can never be FTS5 syntax, caps the input (200 characters, 32 words)
and hands phone-like digits and CJK text back to the LIKE code (`ok == false`). `contacts_fts` has a
column for every free-text field of a contact; the pickers wrap the expression in
`search.ContactBasics` (names, e-mail addresses, phone numbers); the global search (`GET /search`)
and the contact list (`search_all=true`) do not; their LIKE code is `search.ContactAnyFieldLike`, so
both find the same contacts. `search.Available` remembers per database whether an index exists, so
create the tables before the first search on a database.

- **Changing an indexed column** (the list in the trigger's `UPDATE OF`) or **recreating `contacts`,
  `notes` or `activities`** (SQLite drops triggers with the table) means the migration must recreate
  the triggers and rebuild the index: `DELETE FROM contacts_fts;` followed by the
  `INSERT ... SELECT` from migration 34 (contacts) or 29 (notes, activities).
- When the tables do not exist (for example the in-memory test databases built with `AutoMigrate`)
  the search code falls back to the old `LIKE` search, so both paths stay covered by tests;
  `controllers/search_fts_test.go` builds the index from the migration files.

### Running Migrations

Migrations run automatically on startup. To apply or roll back manually:

```sh
cd backend
make migrate-up
make migrate-down
make migrate-status
```

`go test ./database` runs every pair up and down and migrates the databases made by past releases
(`database/testdata/releases/`).

## Models and DTOs

GORM models (`Contact`, `Activity`, etc.) live in `models/`. Request and response types belong to
the module's `adapters/http` and are described by the OpenAPI spec; `models/` holds what GORM
persists.

All models include `UserID uint` for tenant isolation.

## Services

Logic that several modules or jobs share lives in `services/` (sync engines, webhook delivery, the
e-mail outbox, import parsing, backups). The adapters of the modules (persistence, webhooks) call
services; services own multi-step operations (e.g. sending emails). Services receive a `*gorm.DB`
and any needed config, they do not access `*gin.Context`. They never read the wall clock: take
`now time.Time` as a parameter (handlers pass `api.Now(ctx)`, jobs the time taken by the scheduler).
`forbidigo` fails the build on `time.Now`; see [ADR 0009](../adr/0009-injected-clock.md). In tests
pass a fixed time or a `clock.Fake`.

## API contract (huma)

Every JSON endpoint is a [huma](https://huma.rocks) operation in `backend/api` (see
[ADR 0011](../adr/0011-api-contract-with-huma.md)): typed input and output structs, registered with
`api.Register` on the same Gin groups, errors in the AppError format, `api.ValidateBody` for the
project validators. After changing an operation run `make openapi` (writes `docs/openapi.json` and
`frontend/src/types/generated/api.ts`); `make openapi-check` fails when they are stale. Use the
generated types in the frontend instead of hand-written ones. For an update field where leaving it
out keeps the value and `null` removes it, use `api.Optional[T]`.

## Modules (hexagonal)

Converted modules live in `backend/<module>/` (today contacts, notes, activities, reminders,
relationships, photos, trash, users, tokens, webhooks, exports, imports, monicaimport, calendars,
contactsync and admin): a public entry package, and under `internal/` the `domain` (entities and
rules, standard library only), `app` (use cases and the ports they need), and `adapters`
(`persistence` is the only code that imports GORM; `http` registers the huma operations, and keeps
the routes that send bytes or redirects as plain Gin handlers; `webhooks` implements the event
port). `main.go` calls `routes.NewModules(db, cfg, oidcProvider)`, which builds the modules
(`contacts.New(contacts.Deps{...})`), and `routes` registers them. To add behaviour to a converted
module write the use case in `app` (it takes the actor and `now`, and does its writes in one
`Store.InTx`), a port if it needs new persistence, and a thin HTTP handler. See
[ADR 0012](../adr/0012-module-layout.md); the rules are enforced by `make arch`.

## Layering

`services` use `models`, the modules and `controllers` call `services`; never the other way round.
`make arch` (depguard) fails when `models` imports `controllers`, `services` or `gin`, when
`services` imports `controllers` or `gin`, or when anything but `routes` (and `main.go`) imports
`controllers`. See [ADR 0010](../adr/0010-target-layering.md).

## Testing

```sh
make test-go   # or: cd backend && go test ./...
```

Tests use an in-memory SQLite database (auto-migrated). See [Testing](testing.md).
