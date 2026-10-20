# Differences to upstream

How this fork ([omarkohl/meerkat-crm](https://github.com/omarkohl/meerkat-crm), branch `omk`)
differs from [fbuchner/meerkat-crm](https://github.com/fbuchner/meerkat-crm) `main`. It describes
the **current** difference, not the history ([CHANGELOG.md](CHANGELOG.md) has that). When upstream
takes a change, its entry is deleted. For branches, versions and releases see [FORK.md](FORK.md).

Last compared with upstream: `0c2aaab` (after v1.7.0). Full diff:
[`main...omk`](https://github.com/fbuchner/meerkat-crm/compare/main...omarkohl:meerkat-crm:omk)
(about 740 files; roughly 25k of the added lines are tests).

## Merge impact

What can hurt when you merge, or when you look at the diff.

- **New migrations 000024 to 000035.** No upstream migration is changed. Each has a down migration
  and a round-trip test. 000030 adds a column for thumbnail files and copies old thumbnails into
  files at startup; the old column stays.
- **Whole-repo reformat.** Go (`gofmt`), TypeScript, JSON, YAML and Markdown (wrapped at 100
  characters) were formatted once. The commits are in `.git-blame-ignore-revs`. Expect conflicts in
  any file you changed.
- **Frontend toolchain.** Create React App is replaced by Vite, Yarn by npm (`yarn.lock` removed,
  `package-lock.json` added), Node 26 is pinned. The build variable is `VITE_API_URL`, not
  `REACT_APP_API_URL`. Dockerfiles and compose files changed to match.
- **Go 1.27.** `go.mod` is raised from 1.25. `gocron` v1 to v2, `golang-jwt` v4 to v5, `nfnt/resize`
  (archived) to `x/image/draw`. New: `huma/v2`.
- **Backend layout.** Sixteen features (contacts, notes, activities, reminders, relationships,
  photos, trash, users, tokens, webhooks, exports, imports, monicaimport, calendars, contactsync,
  admin) moved from `controllers/` and `services/` into modules under `backend/<module>/`. Wire
  format is unchanged. Error `details` keys of converted endpoints are JSON names (`email`), not Go
  names (`Email`).
- **New environment variables**, all optional: `BLOCK_PRIVATE_URLS`, `METRICS_ENABLED`,
  `METRICS_TOKEN`, `METRICS_ADDR`, `BACKUP_ENABLED`, `BACKUP_TIME`, `BACKUP_DIR`,
  `BACKUP_RETENTION_COUNT`, `AUDIT_LOG_RETENTION_DAYS`, `PROFILE_PHOTO_DIR`, `FAKE_NOW`. The old
  per-feature `*_BLOCK_PRIVATE_URLS` still work.
- **Stricter security headers and CSP** from one place. Setups with the API on another origin need
  `FRONTEND_URL`.
- **SQLite settings.** WAL, `busy_timeout` 5 s, `synchronous=NORMAL`, foreign keys on. One instance
  per database file.

## Overview

Size: S (a day or less), M (days), L (a week or more). "Upstream" is where you can see discussion or
an open pull request. "Separable" means it can be taken without the other rows.

| Area          | Change                                 | Size | Migration | Separable       | Upstream                                                                                                                                 |
| ------------- | -------------------------------------- | ---- | --------- | --------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Users         | Two-factor login (TOTP)                | M    | 28        | yes             | not proposed                                                                                                                             |
| Users         | Sessions and audit log                 | M    | 27        | yes             | not proposed                                                                                                                             |
| Contacts      | Trash                                  | M    | 25        | yes             | not proposed                                                                                                                             |
| Contacts      | Merge duplicate contacts               | S    | –         | needs trash     | not proposed                                                                                                                             |
| Calendar      | Subscribable ICS feed                  | M    | 24, 26    | yes             | not proposed                                                                                                                             |
| Reminders     | Birthday mail once a day               | S    | 35        | yes             | not proposed                                                                                                                             |
| Operations    | Scheduled backups                      | M    | 31        | yes             | not proposed                                                                                                                             |
| Search        | Full-text search (FTS5)                | M    | 29        | yes             | not proposed                                                                                                                             |
| Import        | Preview filter, sort, bulk set         | M    | –         | yes             | not proposed                                                                                                                             |
| Import        | vCard fixes and NOTE field             | S    | 33        | yes             | [PR #251](https://github.com/fbuchner/meerkat-crm/pull/251), [#250](https://github.com/fbuchner/meerkat-crm/issues/250) (vCard 2.1 part) |
| UI            | Date format, placeholder, small fixes  | S    | –         | yes             | not proposed                                                                                                                             |
| Storage       | Thumbnails as files                    | S    | 30        | yes             | not proposed                                                                                                                             |
| Integrations  | Outbox with retries                    | M    | 32        | yes             | not proposed                                                                                                                             |
| Operations    | Prometheus metrics                     | S    | –         | yes             | not proposed                                                                                                                             |
| Security      | One SSRF switch, CSP, session checks   | M    | –         | partly          | not proposed                                                                                                                             |
| Architecture  | Hexagonal modules                      | L    | –         | no (needs huma) | not proposed                                                                                                                             |
| Architecture  | huma API contract, OpenAPI, TS types   | L    | –         | no              | not proposed                                                                                                                             |
| Architecture  | Injected clock, `FAKE_NOW`             | S    | –         | yes             | not proposed                                                                                                                             |
| Build         | Vite, npm, Node 26, strict TS/ESLint   | M    | –         | yes             | not proposed                                                                                                                             |
| Build         | Root Makefile, Docker-free E2E         | M    | –         | yes             | [PR #248](https://github.com/fbuchner/meerkat-crm/pull/248)                                                                              |
| CI            | ESLint, tsc, Prettier, gofmt gates     | S    | –         | yes             | [PR #249](https://github.com/fbuchner/meerkat-crm/pull/249)                                                                              |
| CI            | golangci-lint                          | S    | –         | yes             | [PR #247](https://github.com/fbuchner/meerkat-crm/pull/247)                                                                              |
| CI            | Scans, pinned actions, attestation     | M    | –         | yes             | not proposed                                                                                                                             |
| Testing       | Coverage thresholds, fuzzing, more     | M    | –         | yes             | not proposed                                                                                                                             |
| Docs          | ADRs, API reference test, CONTRIBUTING | S    | –         | yes             | not proposed                                                                                                                             |
| Design drafts | Relationships, "myself", phonebook     | –    | –         | not implemented | [#185](https://github.com/fbuchner/meerkat-crm/issues/185), [#184](https://github.com/fbuchner/meerkat-crm/issues/184)                   |

## User-visible changes

**Two-factor login.** Sign in with a TOTP authenticator app. Recovery codes; an admin can reset a
user. Secrets are encrypted with the key derived from `JWT_SECRET_KEY`, like the CardDAV
credentials. Endpoints under `/users/me/2fa`. Tables and columns: migration 28.

**Sessions and audit log.** Each login is a revocable session ("sign out everywhere" exists). An
admin sees an audit log, kept for `AUDIT_LOG_RETENTION_DAYS`. Deleted users lose access at once.
Migration 27.

**Trash.** Deleted contacts go to a trash page where they can be restored or purged. Adds the
`contact.restored` webhook and a purge marker so CardDAV does not bring a purged contact back.
Migration 25.

**Merge contacts.** Select two or more contacts in the list (or use "Merge with…" on a contact) to
merge them into one. The browser does the work with the ordinary endpoints; there is no merge
endpoint.

**ICS feed.** A private URL that calendar apps subscribe to for birthdays and reminders. The
settings page shows when the feed was last fetched. Migrations 24 and 26.

**CardDAV server.** Conditional writes work: a stale `If-Match`, an `If-None-Match` on an existing
contact and an `If-Match` on a missing one answer `412`; ETags change on every save; a missing
contact or another user's address book answers `404`.

**Birthday mail.** The day of the last birthday mail is stored per user, so a restart on a birthday
does not send it again. Migration 35.

**Scheduled backups.** A daily tarball with a consistent SQLite snapshot and the photo files, kept
`BACKUP_RETENTION_COUNT` times. Admins can create, download and delete backups in the UI. Migration
31 (last-success time of jobs).

**Full-text search.** Contact search uses SQLite FTS5. Migration 29. The header search finds
contacts, notes and activities at once (`GET /search`), grouped, ranked, with the match marked and
the matching field named; it covers every text field of a contact (migration 34), and a results page
(`/search?q=`) has a tab per group with paging.

**Import.** The preview can filter, sort and set a value on many rows at once, and the result lists
skipped rows with the reason. vCard fixes: nameless and partly invalid cards import, vCard 2.1 and
Apple labels work, duplicate phones, emails, URLs and IMs are dropped, and `NOTE` is imported and
synced as a contact note (migration 33). Rows with errors are never imported, and an unreadable card
no longer shifts the cards after it. Importing a deleted contact asks whether to restore it. The
Monica import timeout is longer, and a private Monica URL can be allowed.

**Small UI fixes.** All date inputs and filters use the configured date format, not the browser
locale. One placeholder for empty values. The add-activity dialog preselects the contact and the
edit-activity contact field searches all contacts. Header search clears on navigation and Escape.
The sidebar collapses to icons.

**Thumbnails.** Contact thumbnails are files next to the photos, not base64 in the database. The
database gets smaller and list queries get cheaper. Migration 30; see "Merge impact".

**Integrations.** Webhooks and email go through an outbox with retries and a dead-letter state
(migration 32). Optional Prometheus metrics at `/metrics`, off by default, with an optional token or
a separate listener.

## Security

- `BLOCK_PRIVATE_URLS` is one switch for all outgoing requests (webhooks, CalDAV, CardDAV, Monica)
  and one guard (`backend/netguard`). It ignores `HTTP_PROXY` for those requests.
- Image URLs from users and vCards: ports 80 and 443 only, four fetches at a time, a per-user rate
  limit on the image proxy, which returns a re-encoded JPEG
  ([ADR 0017](docs/adr/0017-fetching-user-supplied-urls.md)).
- One definition of security headers; an API CSP; an E2E guard that fails on CSP violations.
- Private photo permissions, checked close errors, bounded shutdown wait.
- Dependency bumps for known vulnerabilities; `govulncheck` and a Trivy image scan in CI.
- Docker images get current Alpine package fixes at build time.

## Architecture

Decisions are in [docs/adr/](docs/adr/README.md). Rules are machine checks
([ADR 0002](docs/adr/0002-rules-are-machine-checks.md)).

- **Hexagonal modules** ([ADR 0012](docs/adr/0012-module-layout.md)). Each of `contacts`, `notes`,
  `activities`, `reminders`, `relationships`, `photos`, `trash`, `users` (preferences, sessions,
  two-factor, login, registration, password reset, single sign-on), `tokens`, `webhooks`, `exports`,
  `imports`, `monicaimport`, `calendars`, `contactsync` (CardDAV connection settings) and `admin`
  (user management, backups, audit log, system status) has `domain`, `app` (use cases and ports),
  `adapters/persistence` and `adapters/http`. Only the persistence adapter imports GORM. Routes that
  send bytes or redirect stay Gin handlers inside the module. Sync engines, webhook delivery, the
  e-mail outbox and the import parsers stay in `services/`. The CardDAV server is not converted yet.
- **API contract** ([ADR 0011](docs/adr/0011-api-contract-with-huma.md)). huma operations are
  mounted on the existing Gin groups, so middleware and the error format stay. `make openapi` writes
  `docs/openapi.json` and `frontend/src/types/generated/api.ts`; a check fails on drift. Only
  converted endpoints are in the spec.
- **Layering check** ([ADR 0010](docs/adr/0010-target-layering.md)). Import rules for Go and the
  frontend, run as `make arch`.
- **Every query is scoped by `user_id`** ([ADR 0005](docs/adr/0005-user-id-isolation.md)), with an
  E2E isolation test.
- **Injected clock** ([ADR 0009](docs/adr/0009-injected-clock.md)). Code does not read the wall
  clock; a lint rule enforces this. `FAKE_NOW` sets the server clock for tests and previews.
- **`AppError`** ([ADR 0007](docs/adr/0007-apperror.md)) for all error responses.

## Build, CI and testing

- **Frontend:** Vite, npm, Node 26, `noUncheckedIndexedAccess` and related flags, ESLint 9 with
  strict type-checked rules, Vitest 4.
- **Root `Makefile`:** `make dev` runs backend and frontend with no setup; `make precommit` runs the
  commit subject check, format, lint, types, i18n key check, architecture check, OpenAPI check,
  use-case, Markdown link and changelog checks, coverage and E2E. Workflows call the same targets.
  Optional `lefthook`.
- **`make seed`:** adds a demo user (`demo`) and 60 realistic contacts with notes, activities,
  reminders and relationships to the running `make dev` backend. Safe to run again.
- **Tests:** coverage thresholds for Go and Vitest, race detector in CI, fuzzing of the vCard, CSV,
  Monica and ICS parsers, migration round-trip tests, upgrade tests on databases made by past
  releases, CardDAV server and client tests through the HTTP handlers and against Radicale, SMTP and
  OIDC tests against fake servers, Playwright E2E that drive the UI by visible names, invariant
  tests for auth and user isolation of every route and model, and a use-case catalogue
  (`docs/use-cases.yml`, `make use-cases`) that tags the tests.
- **CI hardening:** pinned actions and base images, image scan and attestation, Dependabot for all
  ecosystems, daily with a 7-day cooldown and one group each.
- **Docs:** ADRs, `CONTRIBUTING.md` (replaces `README-developer.md`), an API reference checked
  against the router.

## Fork only

Not meant for upstream.

- Images under `ghcr.io/omarkohl/meerkat-crm*`; workflows and releases run from `omk`; the docs site
  job is skipped.
- `AGENTS.md`, `CLAUDE.md` and `.claude/` for AI-agent work, and `IMPROVEMENTS.md` (open ideas).
- Version scheme `vX.Y.Z-omk.N`.

## Not implemented yet

Design drafts in [design/](design/) that need your input before any code:
[symmetric relationships](design/symmetric-relationships.md) (upstream
[#185](https://github.com/fbuchner/meerkat-crm/issues/185)), [myself](design/myself.md) (upstream
[#184](https://github.com/fbuchner/meerkat-crm/issues/184)),
[phonebook-only contacts](design/phonebook-only-contacts.md).
