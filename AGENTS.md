# Agent rules

Index of the rules agents break most. Details are in the linked files. For autonomous multi-step
runs read [.claude/AUTONOMOUS-RUNS.md](.claude/AUTONOMOUS-RUNS.md) first.

**These rules override consistency with the surrounding code.** If old code breaks a rule, do not
copy it.

## Docs

- [Architecture](docs/development/architecture.md), [backend](docs/development/backend.md),
  [frontend](docs/development/frontend.md), [testing](docs/development/testing.md),
  [contributing and release](docs/development/contributing.md)
- [Decisions (ADRs)](docs/adr/README.md): read them before you re-open a decision. A significant new
  decision gets an ADR in the same commit.
- Docs change in the same commit as the code they describe.
- [DIFFERENCES.md](DIFFERENCES.md) lists how the fork differs from upstream. A change that adds to
  or removes from that difference (feature, migration, API, build, dependency, tooling) updates it
  in the same commit. Remove an entry when upstream has the change. It describes the current state:
  no history and no restating of the changelog.
- A `CHANGELOG.md` entry for a change probably needs a `DIFFERENCES.md` change in the same commit,
  unless there is a very good reason not to.

## Hard limits

These are limits, not targets.

- Comments: at most one per ~50 new lines of code, two lines each. Adding no comment is a normal,
  correct outcome. Delete test: cover the comment; if the code says the same, delete it. Use-case
  tags (`// @001-US1` above a Go test) are not comments in this sense.
- Do the step asked, all of it, nothing beyond it. Unrelated findings go to `PROGRESS.md` under
  "Questions" (in a run) or into your report.

## Failure modes (forbidden)

- Comments that narrate the code, and doc comments that repeat the types.
- Changelog entries that tell a story.
- E2E tests that build URLs with internal IDs or replay raw API payloads.
- Tests that assert internal calls instead of user-visible results.
- Editing an existing migration.
- A query without the `user_id` scope.
- A new UI string that is not in all five locales (en, de, it, es, fr).

## Bad and good

- Bad: ``await page.goto(`/contacts/${contact.ID}`)`` in `frontend/e2e/reminders.spec.ts`. It skips
  the user's path and needs an internal ID. Good: open the contact from the contact list by its
  visible name.
- Bad: `// ErrUnauthorized returns an unauthorized error` in `backend/errors/errors.go`. The comment
  repeats the name. Good: no comment.
- Bad: `c.JSON(http.StatusForbidden, gin.H{"error": "..."})` in `backend/middleware/admin.go`. Good:
  return `apperrors.ErrForbidden("API tokens cannot access admin endpoints")`
  ([ADR 0007](docs/adr/0007-apperror.md)).

## Tests

TDD is mandatory: write the failing test, run it and read the failure, implement the minimum, run it
green, refactor without touching the test. Exceptions: pure styling and fully covered refactors. See
[ADR 0003](docs/adr/0003-testing-strategy.md) and [testing](docs/development/testing.md).

Before a commit run `make precommit` (it includes the E2E tests).

A new route or a model with a `user_id` needs an entry, with its isolation test, in
`backend/routes/invariants_test.go`. A new JSON route is a huma operation (`make openapi`). Tag
tests with the use case they cover (`@001-US1`, `docs/use-cases.yml`).

## Changelog

Update `CHANGELOG.md` under `[Unreleased]` for any user-facing change. The audience is users and
admins of the CRM, so keep it non-technical. Use the
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) sections: `Added`, `Changed`, `Deprecated`,
`Removed`, `Fixed`, `Security`. Dev-only changes go under `Internal`.

**Format**:
`- **Bold lead phrase** (#123): what changed, and, only where it is not obvious, what was wrong before.`

**Hard limits** (limits, not targets):

- One bullet per change. **Two lines of prose, ~40 words, maximum.** No sub-bullets, no second
  paragraph.
- Reference the GitHub issue where one exists, `(#123)`, straight after the lead phrase. Take the
  number from the commit's `fixes #123` / `issue #123` footer, never from the PR number GitHub
  appends to a squashed subject. No issue, no reference.
- Several commits that deliver one feature get **one** bullet.
- `Internal` is **not a second commit history**. It lists only internal changes that are valuable or
  disruptive: roughly **3 entries per release, one line each (~25 words)**. Tests, refactors,
  dependency bumps, lint config and tidy-ups get no entry.

**Never in the changelog**: rationale and trade-offs; how it was implemented; file, function,
library or framework names (outside `Internal`); edge cases; the story of how a bug was found;
future plans.

```
Bad  - **Interrupting a profile slide no longer restarts it**: pressing Next or Prev
       (or swiping again) while a profile was still sliding used to snap the card back
       to the start and replay the whole slide, putting off the arrival a little more
       with every press. Repeated presses now let the first slide finish.
Good - **Interrupting a profile slide no longer restarts it** (#123): pressing Next or
       Prev mid-slide used to replay the animation from the start.
```

Upstream releases (`vX.Y.Z`) are derived from the GitHub release notes of `fbuchner/meerkat-crm`.
Fork releases (`vX.Y.Z-omk.N`) are derived from the commits since the previous release. On release,
rename `[Unreleased]` to the version and date and add the compare link at the bottom.

## Version control

- Conventional commits (`feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`, `build`, `ci`,
  `style`, `revert`), subject at most 72 characters, body says why.
- One commit is one coherent change from the user's view (migration, model, controller, API client,
  UI, i18n and tests together). Each commit passes `make precommit`.
- CI checks subjects and the `[Unreleased]` changelog (`make check-commits`, `make changelog-check`,
  [ADR 0016](docs/adr/0016-changelog-and-commit-rules.md)).
- Issue footer: `fixes #N` when the commit resolves the issue, `issue #N` when it is part of it.
- Review your own diff before you ask for review.
- Never push without explicit permission, each time.
- Unless told otherwise, use the fork `omarkohl/meerkat-crm` for all GitHub actions (issues, PRs,
  `gh` commands), not upstream `fbuchner/meerkat-crm`. Pass `--repo omarkohl/meerkat-crm`.
- With jj, always pass the message and never open an editor: `jj commit -m "msg"`, `jj squash` (add
  `-m "msg"` to avoid the editor when both messages exist), `jj describe -m "msg"`. For diffs use
  `jj diff --git`. Never use `jj split`, `jj resolve` or other interactive commands without
  arguments.
