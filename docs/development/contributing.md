---
title: Contributing
parent: Development
nav_order: 5
---

# Contributing

## Getting Started

1. Fork the repository and clone your fork locally.
2. Follow the setup steps in [Backend](backend.md) and [Frontend](frontend.md).
3. Create a feature branch from `omk` (the fork branch, see [fork.md](fork.md)).

## Development Workflow

1. Make your changes with tests.
2. Run `make precommit` from the repository root: checks the commit subjects, gofmt and Prettier
   formatting of all files, Markdown and docs included (`make format` fixes it), `go vet`,
   `golangci-lint` (the binary must be installed), ESLint, the Markdown link and changelog checks,
   `tsc`, the locale check (`make i18n-check`: a key missing in any of en/de/it/es/fr, or not in
   `en`, fails; unused and duplicate keys only warn), the architecture rules (`make arch`), the
   OpenAPI check, the use-case check, the unit tests with coverage thresholds (Go, Vitest and the
   helper scripts) and the Playwright E2E tests (`make test-e2e`, no Docker needed; narrow it with
   `ARGS=`, see [testing](testing.md)). `make help` lists all targets.
3. Open a pull request against `omk`.

Optional: [lefthook](https://lefthook.dev) can run gofmt, Prettier, ESLint and the locale check on
the staged files before each commit. Install lefthook and run `lefthook install` once; the
configuration is `lefthook.yml`. Nothing installs the hooks automatically, and CI runs the same
checks.

## Code Style

**Backend:** Standard Go formatting (`gofmt`), checked together with `golangci-lint` (config in
`backend/.golangci.yml`: errcheck, staticcheck, gosec, revive, bodyclose, sqlclosecheck).
`make precommit` runs it; the `golangci-lint` binary must be installed (or run
`cd backend && golangci-lint run`). Follow existing controller and service patterns with thin
controllers, logic in services, always scope queries by `user_id`.

**Frontend:** Prettier formatting (`make format` fixes all files), `tsc --noEmit` and ESLint 9 on
`src` (`frontend/eslint.config.mjs`: `typescript-eslint` strict-type-checked, `react-hooks`,
`jsx-a11y`, `eslint-comments`; every rule is an error). Findings that exist today are
bulk-suppressed in `frontend/eslint-suppressions.json`; new violations fail, and fixed ones must be
pruned (details and the reason per rule: [frontend](frontend.md#linting)). An `eslint-disable` needs
a rule name and a description (`// eslint-disable-next-line rule -- why`). `make lint-frontend` is
the check. TypeScript strict mode and MUI for all UI components. All user-facing strings through
`i18next`. Custom hooks for data fetching, not inline `useEffect` + `fetch`.

## Continuous integration

Pull requests run these GitHub Actions workflows, and all must pass:

- **Lint** (`lint.yml`): golangci-lint, ESLint, `tsc`, and a gofmt/Prettier check of all files.
- **Commits** (`commits.yml`): the commit subjects; also reruns when the PR is edited, e.g.
  retargeted.
- **Unit tests** and **E2E tests** (`unit-tests.yml`, `e2e-tests.yml`).
- **Security** (`security.yml`): `govulncheck` and `npm audit` for production dependencies; also
  runs weekly.
- **Docker build and scan** (`docker-build.yml`): builds the three images (all-in-one, backend,
  frontend) without pushing them when a PR touches a Dockerfile, `backend/`, `frontend/` or
  `docker/`, and scans each with Trivy. It fails on fixable HIGH and CRITICAL vulnerabilities;
  unfixed ones are ignored. If a finding cannot be fixed by updating the base image or the
  dependency, add it to `.trivyignore` with a reason and an expiry date. Also runs on pushes to
  `main` and `omk` that touch those paths, and weekly.
- **Docs** (`docs.yml`): builds the Jekyll site on PRs that touch `docs/**` (deploys only from
  `main`). To build locally: `cd docs && bundle install && bundle exec jekyll build`.

Reformat-only commits are listed in `.git-blame-ignore-revs` (GitHub honours it; locally run
`git config blame.ignoreRevsFile .git-blame-ignore-revs`). List a new one there in a follow-up
commit, and only after the reformat commit is pushed (a rebase changes its SHA).

Every CI job runs one `make` target (`lint-go`, `lint-frontend`, `typecheck`, `format-check`,
`test-coverage-go`, `test-e2e-docker`, ...), so a failure reproduces locally with the same command.
Only setup steps (toolchains, Docker image, Radicale) are not in `make`.

Release images (`docker-publish.yml`) are pushed for `linux/amd64` and `linux/arm64` with an SBOM
and SLSA provenance attestation attached
(`docker buildx imagetools inspect <image> --format '{{ json .SBOM }}'`).

**Pinning.** Every GitHub Action is pinned to a full commit SHA with the version in a trailing
comment (`uses: actions/checkout@<sha> # v4.4.0`), and Docker base images to a version tag (the full
version of the program, `<image>:<x.y.z>-alpine`, so Dependabot sees every change). Do not use
`@v4`-style or `latest` references. Dependabot updates the SHAs and tags (and the comment) in its
PRs. To pin a new action, look up the commit of its release tag, for a tag that is annotated the
commit it points to: `git ls-remote https://github.com/<owner>/<repo> 'refs/tags/<tag>*'` and use
the `^{}` line if there is one. Check workflows with
[actionlint](https://github.com/rhysd/actionlint).

Dependabot (`.github/dependabot.yml`, [ADR 0015](../adr/0015-dependency-updates.md)) checks daily
and opens one PR per ecosystem (Go modules, npm, GitHub Actions, Docker, the docs' Ruby gems) for
releases that are at least seven days old. Every `ignore` entry carries a comment with the blocker.

## Releasing

Run `make precommit`, tag the release, then add that release's database to the upgrade tests:

```sh
scripts/seed-release-db.sh vX.Y.Z   # builds the tag's own code, creates and fills a database
git add backend/database/testdata/releases/vX.Y.Z.db
```

`TestEveryReleaseDatabaseMigratesToTheNewestVersionAndStillWorks` migrates every file in
`backend/database/testdata/releases/` to the newest schema and reads and writes through it. The
script needs the network the first time (old dependencies) and writes one file of a few hundred KB.

Several checkouts can run side by side (for example parallel worktrees). `make dev` keeps its data
in `.dev/` of the checkout and takes the first free ports from 8080/7300 (it prints them;
`make seed` finds them). `make test-e2e` uses free ports and a temporary database, so it does not
disturb a running dev server. Set `BACKEND_PORT` / `FRONTEND_PORT` in `dev.env` to pin ports.

## Adding a check

A convention that matters gets a machine check, with a message that says what to do instead and
links the ADR. When you add one:

1. Write the rule and **show that it fails** on a file that violates it: a rule whose file pattern
   matches nothing looks exactly like one that works.
2. Fix the existing violations, or record them as named exemptions with a reason; prefer fixing.
3. Put the reason in the rule's message, not only in a doc.

Exempt a line, not a file, and give a reason (`//nolint:gosec // reason`,
`// eslint-disable-next-line rule -- reason`). Generated artifacts (OpenAPI document, API types) are
committed and checked for drift. Imports that only carry types count in the frontend architecture
rules. Rules that imports cannot express become tests that enumerate
(`backend/routes/invariants_test.go`). When someone trips over the same thing twice, add a paragraph
with the fix to the chapter where it bites.

## Commits and the changelog

- Conventional commits (`feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`, `build`, `ci`,
  `style`, `revert`; an optional `(scope)` and `!`), subject at most 72 characters (Dependabot's are
  not measured), the body says why. One commit is one coherent change from the user's view, and each
  commit passes `make precommit`.
- Name the issue in a footer: `fixes #N` when the commit resolves it, `issue #N` when it is part of
  it.
- A user-facing change adds one bullet to `[Unreleased]` in `CHANGELOG.md`, for users and admins:
  `- **Lead phrase** (https://github.com/omarkohl/meerkat-crm/issues/123): what changed.` at most
  about 40 words, no file or library names. `Internal` entries are rare and short. See
  [AGENTS.md](../../AGENTS.md) and [ADR 0016](../adr/0016-changelog-and-commit-rules.md).
- CI checks the commit subjects of a pull request (`make check-commits` checks the last commit,
  `RANGE=origin/omk..HEAD` a range; also part of `make precommit`; run it after `jj commit` too and
  fix a subject with `jj describe`) and the changelog entries (`make changelog-check`, part of
  `make lint`).

## Pull Requests

- Keep PRs focused with one feature or fix per PR.
- AI assistants are welcome, but you are responsible for the quality of what you submit and must
  understand it well enough to explain it in review. Do not open hands-off vibe-coded PRs you have
  not read and tested; open a feature request instead (same wording as `CONTRIBUTING.md`).
- Describe what changed and why, not how.

## Reporting Issues

Open an issue on [GitHub](https://github.com/omarkohl/meerkat-crm/issues/new/choose). Try to include
steps to reproduce for bugs. Use the feature request template for new ideas.
