# Autonomous runs

Rules for Claude when it works through a multi-step plan without the user, mostly in cloud sessions
(`CLAUDE_CODE_REMOTE=true`). A run is active while `.claude/run/PLAN.md` exists. Hooks in
`.claude/hooks/` enforce the push rules and keep the run going; this file is the reference they
point to.

(`AGENTS.md` holds the everyday agent rules; this file adds the run rules. The SessionStart hook
prints this file in cloud sessions.)

## Starting a run

1. Copy `.claude/run-template/` to `.claude/run/`, fill in `PLAN.md` with the user: goal, what to
   read first, decisions settled by the user, decisions settled by Claude, and numbered steps with
   `- [ ]` checkboxes.
2. Each step is one semantically coherent commit, end-to-end from the user's view (model +
   migration + controller + API client + UI + i18n + tests together). `main` must stay deployable
   after every step.
3. Step 0 is always the environment check; the last step deletes `.claude/run/` in its own commit.

## Orchestration

The main session is the orchestrator. Per step:

1. Hand the step to the `implementer` agent (reuse the same instance across steps).
2. Hand the resulting commit to a **fresh** `reviewer` agent.
3. Send the review to the implementer; it fixes or rejects each finding and amends.
4. Cloud: create the step branch, push, open the PR (see below). Update `## Stack` and
   `## Next step` in PROGRESS.md.
5. Go on with the next step. Stop only when all steps are ticked or a blocker is recorded under
   `## Blocked`.

## Checks

Run from the repository root. All must pass before a commit:

- `make precommit`: gofmt and Prettier check on all files, `go vet`, golangci-lint, ESLint, Markdown
  link and changelog checks, `tsc`, i18n check, architecture rules, OpenAPI check, script tests,
  use-case check, Go and Vitest tests with coverage thresholds and `make test-e2e` (Playwright
  against a local stack without Docker, `scripts/e2e-stack.mjs`). Use `E2E_SKIP_BUILD=1` to reuse
  builds and `ARGS=e2e/contacts.spec.ts` to narrow, when you run `make test-e2e` alone.
- If `npx playwright install chromium` is blocked, look for preinstalled browsers (e.g.
  `/opt/pw-browsers`) and point a throwaway, **uncommitted** Playwright config at them via
  `launchOptions.executablePath`. Record the workaround in PROGRESS.md. If no browser works, record
  it and rely on CI for E2E.

## Project rules that reviews check

- **Tests first**: red → green → refactor. Backend tests use in-memory SQLite like the existing
  `*_test.go`. E2E uses roles and labels, no test ids.
- **User isolation**: every query is scoped to the authenticated user's `user_id`. Add a case to
  `frontend/e2e/isolation.spec.ts` or a Go test for new resources.
- **Migrations**: new numbered pair in `backend/database/migrations` (`up` and a working `down`).
  Never edit an existing migration.
- **Validation**: request bodies via `api.ValidateBody` in the input's `Resolve` method. Errors as
  `*apperrors.AppError`.
- **Transactions** for multi-entity writes; deletes clean up dependants.
- **i18n**: every new UI string in all five locales (`frontend/src/i18n/locales/`, and
  `backend/i18n/locales/` for backend strings): en, de, it, es, fr.
- **API**: a new JSON endpoint is a huma operation in its module; run `make openapi`; the frontend
  derives types from `src/types/generated/api.ts` and calls `src/api/client.ts`; update
  `docs/api-reference.md` for behaviour. A new route or model with a `user_id` needs its entry and
  isolation test in `backend/routes/invariants_test.go`. Tag tests with a use case from
  `docs/use-cases.yml`.
- **Scope**: do the step, all of it, nothing beyond it. Unrelated findings go to PROGRESS.md
  `## Questions`.

## Commits

- Conventional commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`, ...; subject at
  most 72 characters, checked by `make check-commits`). Message as short as possible, as long as
  necessary, focus on **why**.
- One commit per step; tick the step in PLAN.md and update PROGRESS.md in the same commit.
- Cloud: use `git`. Locally: use `jj`.

## Pushing and PRs (cloud only)

- Branch per step: `<run-name>/<NN>-<slug>`, based on the previous step's branch (stacked). Step 1's
  base is `omk`.
- Never push to `main`. Never merge PRs or delete branches: that is for the user.
- `gh pr` uses GraphQL, which the cloud GitHub proxy blocks. Use REST:
  `gh api repos/{owner}/{repo}/pulls -f title=... -f head=<branch> -f base=<base> -f body=...`
- PRs go to the repository the session was cloned from (`origin`) only.
- PR body: what and why in a few lines, check results, "For review" items of this step.
- Locally during a run: push nothing, open no PRs. Ask the user.

## Decisions

When PLAN.md, the code and the docs do not settle a question:

1. If it is reversible and local to the step, take the simplest option that fits the codebase, note
   it under `## Decisions` in PROGRESS.md and in the PR body.
2. If it changes data, the API contract, security or a later step, do not guess: write it under
   `## Blocked`, commit, push, stop.

## PROGRESS.md

Sections: `## Next step`, `## Stack` (step: branch, PR), `## Log` (one bullet per step: what
changed, check results, surprises), `## Decisions`, `## For review` (things the user must look at,
e.g. UI choices), `## Questions`, `## Blocked`. Keep `## Blocked` empty unless the run cannot go on.
