---
title: Testing
parent: Development
nav_order: 4
---

# Testing

## Make targets

`make test` (script tests, use-case check, Go and Vitest tests), `make test-coverage` (Go and Vitest
tests with coverage thresholds), `make test-e2e` (Playwright against a local stack),
`make precommit` (all checks including E2E). Targets that need dependencies depend on `install-go`
and/or `install-frontend` (`install` runs both), so a fresh clone works. CI runs the same targets
(`test-coverage-go GO_TEST_FLAGS=-race`, `test-coverage-frontend`, `test-carddav`,
`test-e2e-docker`).

## Running one test

| Suite      | By file                                    | By name                                              | By line                                           |
| ---------- | ------------------------------------------ | ---------------------------------------------------- | ------------------------------------------------- |
| Go         | `cd backend && go test ./notes/`           | `cd backend && go test ./notes/ -run TestName -v`    | no line syntax; use `-run` with the test function |
| Vitest     | `cd frontend && npm test src/auth.test.ts` | `npm test src/auth.test.ts -- -t "part of the name"` | no line syntax; use `-t`                          |
| Playwright | `make test-e2e ARGS=e2e/contacts.spec.ts`  | `make test-e2e ARGS='-g "part of the title"'`        | `make test-e2e ARGS=e2e/contacts.spec.ts:42`      |

`E2E_SKIP_BUILD=1 make test-e2e ARGS=...` reuses the previous builds.

## Strategy

Decision: [ADR 0003](../adr/0003-testing-strategy.md). Test behaviour, not implementation: a test
fails when a user's outcome changes, and ideally only then.

### Tiers

| Tier        | Share    | Scope                                                                                                                                                                |
| ----------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Integration | the bulk | Business logic through its real entry points (HTTP handlers, use cases) against a real SQLite database. Mock only what you cannot run locally (Resend, remote OIDC). |
| E2E         | few      | Important user workflows in Playwright. Quality over quantity.                                                                                                       |
| Unit        | rare     | Complex isolated functions only (vCard/CSV parsers, ICS folding). Never duplicate integration coverage.                                                              |

Integration tests:

- Keep the real database cheap: migrate a template database once, copy it per test.
- Verify results through a read surface, in this order: the matching read endpoint, then repository
  reads, then raw rows as a last resort.
- Scheduler jobs do not run implicitly in tests; tests trigger them, then assert.

Docs: `make doc-links` (part of `make lint` and a CI step) checks that every repo-relative Markdown
link points at an existing file and every `#anchor` at a heading of that file. External links are
not followed. A link to `page.html` is fine when `page.md` exists: the published Jekyll site (plain
Jekyll, no relative-links plugin) serves pages as `.html`, so links between `docs/` pages use that
spelling and `.md` is used elsewhere.

Releases and optional services:

- `backend/database/testdata/releases/<tag>.db` are databases made by released versions
  (`scripts/seed-release-db.sh <tag>`, see [contributing](contributing.md#releasing)); a test
  upgrades each to the newest schema and uses it.
- The Radicale scenarios are opt-in locally (`make test-carddav`). With `CI=true` and
  `MEERKAT_CARDDAV_IT` unset the tests fail rather than skip; a job that runs without Radicale sets
  `MEERKAT_CARDDAV_IT=off` on purpose.

Use cases:

- `docs/use-cases.yml` lists the use cases (`NNN-USn`, actor, priority, status). Add one when you
  add a user-visible capability.
- Every E2E test title ends with the IDs it covers (`@004-US3`); a Go test carries a `// @004-US3`
  comment line directly above its function (a tag in a comment, not in the name: Go names cannot
  hold the ID; stack several tags; a tag anywhere else fails the check). `make use-cases` (part of
  `make test` and `make precommit`, and a CI step) fails on an unknown ID or an E2E test without
  one, and lists the use cases no test covers.

E2E tests:

- Act like a user: click visible elements, follow links, type into labelled fields. Use role, label
  and visible text locators, not CSS or test IDs.
- Never construct URLs with internal IDs (`/contacts/${id}`) and never replay raw API payloads. Open
  a contact with `openContact(page, contact)` from `fixtures.ts` (search the list, click the name).
  ESLint (`eslint-plugin-playwright`, `make lint-frontend`) fails on a template-literal `goto` and
  on `page.locator(...)`.
- Specs that change instance-wide state are named `*.serial.spec.ts`: they run after the parallel
  project, one file at a time (see `playwright.config.ts`). To run just one of them use
  `make test-e2e ARGS='--no-deps e2e/backups.serial.spec.ts'`; the project it depends on is skipped.

### Quality guardrails

- A test that breaks on an internal rename with no user-visible change is a bad test: rewrite or
  delete it.
- No assertions on call counts of internal helpers; no reaching into unexported state.
- Factories build minimal valid entities; tests override only the fields they care about. If a test
  sets 12 fields, fix the factory.
- No shared state between tests.

### TDD (mandatory for agents)

1. Find or add the use case; write the failing test.
2. Run it and confirm the failure output: this proves the test tests something.
3. Implement the minimum to pass.
4. Run again; confirm green.
5. Refactor without touching the test.

Exceptions: pure styling changes, and refactors that tests already cover fully. Write no test for
those (no tests that assert layout or spacing).

## Invariant tests

`backend/routes/invariants_test.go` enforces three rules from the Gin engine and the model files:

- A mutating route answers `401` to an anonymous request unless it is in `publicRoutes` (with a
  reason).
- Every mutating route that needs a login is in `isolation`, with the test that shows another user
  (or a non-admin) cannot use it. Public routes are exempt. The test is checked to be a top-level
  `Test*` function in the named file.
- Every struct in `models/*.go` with a `UserID` is in `covered`, with a Go test or a running title
  in `frontend/e2e/isolation.spec.ts` (a `test.skip` does not count), or in `noAPI` with the reason
  no endpoint reaches it.

A new route or model fails these until you add its entry and the test behind it.

## Backend Tests

### Running Tests

```sh
cd backend
go test ./...
```

CI runs `make test-coverage-go GO_TEST_FLAGS=-race` (the race detector needs cgo and a C compiler;
the SQLite driver itself is pure Go). Run the same locally to reproduce a race, and
`go tool cover -func=coverage.out | tail -1` for the total. Coverage is measured across all packages
(`-coverpkg=./...`), so a module's tests count for its `internal/` code.

### Coverage

The Unit Tests workflow prints the total coverage of the Go and Vitest suites in the job summary and
uploads the full reports (`go-coverage`, `frontend-coverage`) as workflow artifacts for 14 days. No
third-party service is involved. Locally: `make test-coverage` (part of `make precommit`) writes
`backend/coverage.out` and `frontend/coverage/`.

**Thresholds.** Coverage may not fall: Vitest fails below `coverage.thresholds` in
`frontend/vite.config.ts`, and `scripts/check-go-coverage.mjs` fails when the total Go coverage is
below `GO_COVERAGE_MIN` in the `Makefile`. Both sit at the value of the day they were introduced,
rounded down. When coverage rises, raise them in the same commit; never lower them to make a change
pass, add tests instead. The one exception is a tool change that measures differently: Vitest 4
counts v8 coverage per source statement, so the Vitest thresholds were set again on that day.

### Migrations

`TestMigrationsRoundTripStepByStepOnSeededDatabase` applies all migrations, fills the main tables,
then rolls back one migration at a time (no table may lose rows, the schema of every version is
recorded), checks the database is empty, and applies the migrations one at a time again, comparing
each version's schema with the recorded one. `TestMigrationsUpKeepsRowsSeededAtOlderVersions` seeds
databases at older versions and checks that the rows survive the migrations up to the newest. Every
new migration needs a working `down`, or these tests fail. Add a statement for a new table to
`seedStatements`. The one known exception is the down migration of 000008, which leaves three
`contacts` columns behind (SQLite limitation); the test ignores them below version 8.

### CardDAV integration (Radicale)

The contact sync tests (and the other tests of `./services/` that use the same harness) also run
against a real Radicale server when `MEERKAT_CARDDAV_IT=1` is set; CI does this in the
`CardDAV sync (Radicale)` job. Locally:
`docker compose -f docker-compose.carddav-test.yml up -d --wait radicale`, then `make test-carddav`.
The Radicale image tag is pinned in the compose file and in `unit-tests.yml`; change both together.

### Fuzzing

Go native fuzz targets cover the parsers that read untrusted input:

| Target                   | Package    | What it feeds                                                                                                                                |
| ------------------------ | ---------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `FuzzVCardToContact`     | `carddav`  | vCard bytes through the decoder, `VCardToContact`, and back through `ContactToVCard` (CardDAV, VCF import)                                   |
| `FuzzDecodePhoto`        | `carddav`  | untrusted image bytes (upload, vCard PHOTO, remote photo) through the decoder behind `SaveContactPhoto`, crop and resize; nothing is written |
| `FuzzCSVImport`          | `services` | CSV through `ParseCSV`, `SuggestColumnMappings`, `BuildContactFromRow`, validation                                                           |
| `FuzzImportValueParsers` | `services` | birthday, phone, gender and circle parsers                                                                                                   |
| `FuzzMonicaMapping`      | `services` | arbitrary JSON decoded into the Monica types and run through every mapper                                                                    |
| `FuzzICSText`            | `services` | ICS text escaping and line folding                                                                                                           |
| `FuzzBuildICSFeed`       | `services` | stored names, birthdays and reminder texts through `BuildICSFeed`; the feed must stay well formed                                            |

`go test ./...` runs every target once over its seeds (the `f.Add` values and the files in
`testdata/fuzz/<Target>/`), so they double as ordinary regression tests. To fuzz one for real:

```sh
make fuzz FUZZ_PACKAGE=./services FUZZ_TARGET=FuzzCSVImport FUZZ_TIME=60s
```

The `Fuzz` workflow does this for every target (30 s each by default) nightly and on demand
(Actions, Fuzz, Run workflow; the time per target is an input). It is not part of pull request
checks. When a target fails, the failing input is uploaded as the artifact
`fuzz-failing-input-<Target>`. Copy the file into `backend/<package>/testdata/fuzz/<Target>/`, fix
the bug, and commit the file with the fix: from then on `go test` replays it. Add a plain unit test
as well when the bug has a name.

### Test Database

Tests use an in-memory SQLite database that is auto-migrated before each test. No external
dependencies required.

### Writing Tests

Use the test helpers in the `_test` files alongside each package. Set up a test DB with
`database.NewTestDB()` (or equivalent), create a Gin test context, and call controller functions
directly. Assert on the `httptest.ResponseRecorder`.

## Frontend Tests

Component and hook tests run with Vitest (`cd frontend && npm test`, or `make test` for everything).
Playwright runs the integrated E2E tests.

### Setup

```sh
cd frontend
npx playwright install  # install browsers (first time only)
```

The E2E tests expect the full application on `E2E_BASE_URL` (default `http://localhost:7300`).
`global-setup.ts` seeds a test user before the suite runs.

### Running E2E Tests

Without Docker, `make test-e2e` builds both apps and runs Playwright against a local stack
([scripts/e2e-stack.mjs](../../scripts/e2e-stack.mjs)). `E2E_SKIP_BUILD=1` reuses builds;
`ARGS=e2e/contacts.spec.ts` narrows the run. The stack uses free ports and a temporary database, so
it can run next to a dev server and next to other checkouts. Against a stack you started yourself:

```sh
npm run test:e2e           # headless
npm run test:e2e:headed    # with browser visible
npm run test:e2e:ui        # Playwright UI mode
npm run test:e2e:debug     # debug mode
```

### Writing E2E Tests

Tests live in `frontend/e2e/`. Use the shared fixtures from `fixtures.ts` for login/logout helpers
and the `TEST_USER` credentials from `global-setup.ts`. Group tests with `test.describe` and keep
each test independent.
