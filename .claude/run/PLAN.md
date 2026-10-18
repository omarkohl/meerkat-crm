# Run plan: engineering-principles

Goal: make the fork follow the engineering principles in
`.claude/plans/engineering-principles-reference.md` (§ numbers below refer to it), and move their
content into ADRs, `AGENTS.md` and `docs/development/` so the reference can be deleted at the end.
Every step leaves `omk` deployable. To start the run, copy this file to `.claude/run/PLAN.md` (after
the `first-sprint` run is finished) and copy `.claude/run-template/PROGRESS.md` next to it.

Read first: `.claude/plans/engineering-principles-reference.md`, `.claude/AUTONOMOUS-RUNS.md`,
`docs/development/*.md`, `Makefile`, `.github/workflows/`, `backend/.golangci.yml`, `FORK.md`.

## Evaluation (2026-10-16)

| #   | Principle                  | Status  | Evidence                                                                                                                                                                                                                      |
| --- | -------------------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | One command, everywhere    | Partial | Root `Makefile` with all base targets and `precommit`; `make dev` per checkout. Missing: `arch`, `test-coverage`, E2E not in a gate, CI calls raw commands, no seed data, no doc for "run one test", no target prerequisites. |
| 2   | Wrong code fails to build  | Partial | `tsc strict`, golangci-lint with line-level `//nolint` + reason, Prettier/gofmt. Missing: `noUncheckedIndexedAccess`, ESLint is CRA `react-app` (not type-aware strict), format check on changed files only, no custom rules. |
| 3   | Hexagonal architecture     | Missing | Controllers query GORM directly (`c.MustGet("db")`, 26 files). No domain layer, no ports, no module entry files, no module-graph checker.                                                                                     |
| 4   | Test behaviour             | Partial | Go tests on real SQLite, isolation E2E, migration round-trip and older-version seed tests, E2E on the Docker image in CI, Radicale in CI. Missing: coverage thresholds, use-case catalogue, E2E builds URLs with IDs (8x).    |
| 5   | Determinism                | Missing | 84 `time.Now()` calls in non-test Go code, no injected clock, no fake clock, no lint ban. Few moving parts: yes (SQLite, one image).                                                                                          |
| 6   | Dependency updates         | Partial | Dependabot for all ecosystems, grouped. Missing: daily schedule, cooldown, one group per ecosystem (major updates are not grouped), issue comments on pins.                                                                   |
| 7   | Small coherent commits     | Partial | Conventional commits, push rules in `AUTONOMOUS-RUNS.md`. Missing: issue footers, subject-length check, jj gotchas.                                                                                                           |
| 8   | Docs with the code         | Partial | `docs/` and `design/` in repo. Missing: ADRs, committed `AGENTS.md` (`CLAUDE.md` is gitignored), user changelog.                                                                                                              |
| 9   | Instructions agents follow | Partial | `AUTONOMOUS-RUNS.md` has concrete rules. Missing: hard limits, bad/good examples, named failure modes, "rules beat surrounding code".                                                                                         |

## Settled before the run (the user, 2026-10-16)

- Fork only. The user accepts the conflicts on upstream rebases from the wide changes (steps 5, 7,
  11–13). No waiting for upstream; offering it upstream may come later.
- Agent rules go in a committed `AGENTS.md`.
- Step 5 (full reformat) stays, against the earlier "no repo-wide reformat".
- `precommit` includes `test-e2e`.
- No hexagonal layers in the frontend (it stays simple); only the dependency-cruiser rules of
  step 10.
- OpenAPI with `huma` (code-first), converted module by module with the hexagonal steps.

## Settled by Claude (the user may override before launch)

- Order: docs and gates first, then coverage thresholds, then the clock, then the API contract and
  the architecture refactor module by module. The refactor needs the thresholds and the existing E2E
  suite as safety nets. Test-quality steps (16–18) come after it, so they use the new layout.
- New checks start with a baseline (named exemptions with a reason), then each later step removes
  exemptions. Every new rule is shown to fail on a violation first.
- Go module-graph checks: `depguard` and `forbidigo` in golangci-lint (no new tool). Frontend:
  `dependency-cruiser`.
- ADRs in `docs/adr/NNNN-slug.md` (status, date, context, decision, consequences).
- Where the reference content goes (the step that writes it in brackets):

  | Reference section | Destination                                                               |
  | ----------------- | ------------------------------------------------------------------------- |
  | Overview, §2      | ADR "rules are machine checks with fix messages" (1); `AGENTS.md` (2)     |
  | §1                | `docs/development/contributing.md` (3), `AGENTS.md` (2)                   |
  | §3                | ADRs for layering (10) and module layout (12); `architecture.md` (10, 12) |
  | §3 contracts      | ADR huma (11)                                                             |
  | §4                | ADR "testing strategy" (1); `docs/development/testing.md` (1, 8, 15–18)   |
  | §4 TDD, §9        | `AGENTS.md` (2)                                                           |
  | §5                | ADR clock (9); ADR SQLite and few moving parts (1)                        |
  | §6                | ADR dependency updates (20)                                               |
  | §7                | `AGENTS.md` (2), `contributing.md` (21)                                   |
  | §8                | ADR 0001 (1), `AGENTS.md` (2), ADR changelog (21)                         |

## Steps

- [x] 0. **Environment check** (no own commit; record in PROGRESS.md with step 1): `make precommit`
      green, whether `make test-e2e` runs.
- [x] 1. **ADR folder (§8)**: `docs/adr/` with ADR 0001 "record decisions" and a README index. ADRs
      for the principles: "rules are machine checks with fix messages" (overview, §2) and "testing
      strategy" (§4 tiers, guardrails, TDD; details go to `docs/development/testing.md`). Backfill
      ADRs for decisions already in place: SQLite + WAL and few moving parts (§5), `user_id`
      isolation, never edit a migration, `AppError`, five locales. Link from
      `docs/development/architecture.md`.
- [x] 2. **`AGENTS.md` as index (§8, §9)**: committed file with the rules agents break most, links
      to `AUTONOMOUS-RUNS.md`, testing, architecture, ADRs, release docs. Hard limits (comments: at
      most one per ~50 new lines, two lines each; "zero is a normal outcome"), named failure modes,
      two or three bad/good examples from this codebase, "these rules override consistency with
      surrounding code", jj non-interactive commands, push policy, issue footers
      (`fixes #N`/`issue #N`). Remove `CLAUDE.md` from `.gitignore` and commit a `CLAUDE.md` that
      only contains `@AGENTS.md`, so Claude Code loads it.
- [x] 3. **One command (§1)**: targets declare prerequisites (`test: install` etc.); add
      `test-coverage`, `arch` (empty placeholder until step 10), `seed` (demo user and contacts for
      `make dev`); add `test-e2e` to `precommit` and update the "Checks" section of
      `AUTONOMOUS-RUNS.md`. Document how to run one test by file, line and name for Go, Vitest,
      Playwright in `docs/development/testing.md`.
- [x] 4. **CI calls make targets (§1)**: every workflow job runs one `make` target (`lint-go`,
      `lint-frontend`, `typecheck`, `format-check`, `test-go`, …); split `lint` into sub-targets as
      needed. Lockfile-only installs stay.
- [x] 5. **Full format check (§2)**: one reformat commit for all Go and TS files, add its revision
      to `.git-blame-ignore-revs`, then `format-check` checks all files. Remove the `BASE` logic.
- [x] 6. **TypeScript strictness (§2)**: enable `noUncheckedIndexedAccess`, `noImplicitOverride`,
      `noPropertyAccessFromIndexSignature`; fix the errors.
- [x] 7. **ESLint flat config (§2)**: ESLint 9 flat config with `typescript-eslint`
      `strict-type-checked`, `react-hooks`, `jsx-a11y`, `eslint-comments` (`require-description`,
      `no-unlimited-disable`). All rules `error`. Replace the file-wide disable in
      `service-worker.ts` with line-level ones. Baseline remaining findings with named exemptions.
- [x] 8. **Coverage thresholds (§4)**: Vitest `coverage.thresholds` and a Go threshold check on
      `coverage.out` (script), both at the current value rounded down. `make test-coverage` in
      `precommit` and CI.
- [x] 9. **Injected clock (§5)**: ADR. Go: a `clock` package (`Clock` interface, real and fake);
      `now` taken once per request in middleware and per job in the scheduler, then passed down.
      Replace all `time.Now()` outside `main.go`, middleware and the clock package. `forbidigo` bans
      `time.Now` elsewhere with a message that links the ADR. Fake clock behind an explicit flag
      (`FAKE_NOW=...`, not the build mode). Frontend: ESLint ban on `Date.now()` and `new Date()`
      without arguments outside one `now` helper.
- [x] 10. **Module-graph checks, baseline (§3)**: ADR for the target layering. Go `depguard` rules:
      `models` imports no `controllers`/`services`/`gin`; `services` imports no `controllers`/`gin`;
      nothing imports `controllers` except `routes`. Frontend `dependency-cruiser`: no-circular,
      no-unresolvable, pages call `src/api` only through hooks, nothing imports `src/api/client.ts`
      except `src/api/*`. Show each rule fails on a test violation. `make arch` runs them; add to
      `precommit` and CI.
- [x] 11. **API contract setup with huma (§2, §3)**: ADR. Start with a spike; if a check below
      fails, record it under `## Blocked` and stop. Mount huma on the existing Gin groups with
      `humagin`, so Gin middleware (auth, rate limit, request ID) still runs. Keep the `AppError`
      JSON format for huma errors. Port the 6 custom validators (`phone`, `birthday`,
      `strong_password`, `unique_circles`, `no_at_sign`, `safeurl`). Convert one small endpoint to
      prove the chain. `make openapi` writes the committed spec (OpenAPI 3.1); `openapi-typescript`
      generates `frontend/src/types/generated/api.ts`. CI regenerates both and fails on a diff.
- [x] 12. **Hexagonal pilot: contacts (§3)** (split in three parts, see PROGRESS.md, ADR 0012):
  - [x] 12a. Layout, ADR, depguard rules, composition root; archive, unarchive, delete, circles
  - [x] 12b. Get, create and update one contact
  - [x] 12c. Lists (search, random, birthdays); delete hand-written frontend contact types and the
        Gin handlers Full scope: ADR with folder layout (`domain/`, `app/` use cases with ports,
        `adapters/http`, `adapters/persistence`) and a composition root in `main.go`. Move contact
        logic: entity and rules to domain, use cases take actor + `now` and run writes in one
        transaction, GORM code to a repository. HTTP handlers become huma operations with typed
        input/output (no `gin.H`) and only map HTTP to the use case. The frontend uses the generated
        contact types; delete the hand-written ones. Tests through the HTTP handler on real SQLite
        stay green. Tighten `depguard` for the migrated module (`domain-stays-pure`,
        `use-cases-depend-on-ports`, `repositories-stay-in-persistence`).
- [ ] 13. **Hexagonal: remaining modules (§3)**, one commit per module, same pattern as step 12
      (huma handlers, generated frontend types); remove each module's exemptions from the arch
      rules:
  - [x] 13a. notes
  - [x] 13b. activities
  - [x] 13c. reminders (scheduler job deferred to 13i, see PROGRESS)
  - [x] 13d. relationships and graph
  - [x] 13e. photos and thumbnails
  - [x] 13f. trash
  - [x] 13g. users, sessions, TOTP, OIDC, password reset (split in five, see PROGRESS Decisions):
    - [x] 13g1. user preferences: language, date format, custom field names, enabled contact fields
    - [x] 13g2. sessions (list, revoke, sign out everywhere) and change password
    - [x] 13g3. two-factor (TOTP) endpoints
    - [x] 13g4. register, login, logout, password reset, password strength, current user (split):
      - [x] 13g4a. password strength and password reset (public routes, rate limit)
      - [x] 13g4b. register and `GET /users/me`
      - [x] 13g4c. login with the second step and logout
    - [x] 13g5. OIDC login and callback (redirects; stay Gin handlers in the module)
  - [x] 13h. API tokens and ICS feeds
  - [ ] 13i. webhooks and email outbox (incl. the daily reminder e-mail job; split, see PROGRESS):
    - [x] 13i1. webhook management endpoints (list, create, get, update, delete, test, deliveries)
    - [x] 13i2. daily reminder e-mail job (into the reminders module)
    - [x] 13i3. email outbox delivery engine: stays in `services` (decision in PROGRESS, no code
          change)
  - [x] 13j. import (CSV, VCF, Monica) and export (split, see PROGRESS):
    - [x] 13j1. export (CSV and VCF downloads)
    - [x] 13j2. CSV and VCF import (upload, preview, confirm; session manager)
    - [x] 13j3. Monica import (connect, fetch, status, preview, confirm, cancel)
  - [ ] 13k. CardDAV sync settings and contact/calendar sync (the CardDAV protocol endpoints stay
        plain Gin: they are not JSON)
  - [ ] 13l. backups, audit log, system status
- [ ] 14. **Contract complete (§2)**: a test fails if a JSON route under `/api/v1` is not a huma
      operation. Delete the remaining hand-written API types and `ValidateJSONMiddleware`.
      `docs/api-reference.md` links to the spec.
- [ ] 15. **Invariant tests (§3, §4)**: a Go test enumerates every route from the Gin engine and
      fails if a mutating route has no auth middleware or no entry in the isolation test table. A
      test enumerates every model with `user_id` and fails if `isolation.spec.ts` or a Go isolation
      test does not cover it.
- [ ] 16. **E2E quality (§4)**: replace ``page.goto(`/contacts/${id}`)`` (8x) and CSS/test-id
      locators (7x) with user navigation and role/label locators. Add `eslint-plugin-playwright`
      plus a rule that bans template-literal `goto`. Move specs that change global settings into a
      serial project after the parallel one.
- [ ] 17. **Use-case catalogue (§4)**: `docs/use-cases.yml` (ID `NNN-USn`, actor, priority, status).
      Tag E2E titles (`@001-US1`) and Go test names. A script in `make test` fails on unknown IDs
      and on E2E tests without ID, and prints uncovered use cases. Add to CI.
- [ ] 18. **Test what you ship (§4)**: release-seeded databases in
      `backend/database/testdata/releases/<tag>.db`; a test migrates each to the newest version and
      does basic reads and writes; release doc gets the step to add one. Radicale and other optional
      tests fail when `CI=true` and the service is not configured (no silent skip).
- [ ] 19. **Doc link check (§2)**: offline Markdown link check (repo-relative links and anchors) in
      `make lint` and CI.
- [ ] 20. **Dependency updates (§6)**: ADR. Dependabot `daily`, `cooldown: {default-days: 7}`, one
      group per ecosystem for all update types. Every `ignore` gets a comment with the blocking
      issue. Pin floating Dockerfile base-image tags so Dependabot sees them.
- [ ] 21. **Commit checks and changelog (§7, §8)**: CI job checks PR commit subjects (conventional
      prefix, ≤ 72 chars). `CHANGELOG.md` for users: one bullet per change, issue reference, length
      limit checked by a script. ADR for the changelog; commit rules (§7) in
      `docs/development/contributing.md` and `AGENTS.md`.
- [ ] 22. **Docs**: README, `docs/development/*`, `AGENTS.md` reflect the new targets and layout.
      Full `make precommit`.
- [ ] 23. **Finish**: check each row of the destination table: the content is in its destination.
      Summary and "For review" list in PROGRESS.md. Then delete `.claude/run/` and
      `.claude/plans/engineering-principles*.md` in their own commit.

## Next steps

After this run, pick further work from `IMPROVEMENTS.md` (open items only).
