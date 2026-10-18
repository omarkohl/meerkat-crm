# Run plan: coverage-and-search

Goal: fix the contact search when an activity is edited
([#140](https://github.com/omarkohl/meerkat-crm/issues/140)), close the regression-test gaps of
[#107](https://github.com/omarkohl/meerkat-crm/issues/107), and add a global full-text search across
contacts, notes and activities ([#118](https://github.com/omarkohl/meerkat-crm/issues/118)). Every
step leaves `main` deployable.

Read first: `.claude/AUTONOMOUS-RUNS.md`, `AGENTS.md`, `docs/development/*.md`,
`docs/adr/0003-testing-strategy.md`, `docs/adr/0005-user-id-isolation.md`,
`docs/adr/0011-api-contract-with-huma.md`, `backend/search/search.go`,
`backend/database/migrations/000029_add_fts_search.up.sql`, `frontend/src/App.tsx` (header search).

## Settled before the run (the user, 2026-10-18)

- Scope and order: #140, then #107, then #118. #109 (merge contacts) waits for the next run.
- #107: no GitHub writes. The audit and the status of every checkbox go into PROGRESS.md under
  `## Coverage audit (#107)`; the user copies them into the issue.
- #118 header search: contacts first, then the other results grouped by type, with the matched text
  highlighted. Only the top results of each group. A link opens a page with all results. Follow the
  usual best practice for this kind of search (see step 12).
- #118 indexed contact fields, in addition to names, emails and phones: CardDAV note (`note`),
  addresses (`addresses` and legacy `address`), circles, and the other free-text fields
  (`how_we_met`, `food_preference`, `work_information`, `contact_information`, `organization`,
  `department`, `job_title`, `role`, `middle_name`, custom field values).

## Settled by Claude (the user may override before launch)

- #140 cause: `EditTimelineItemDialog` filters a preloaded `allContacts` list (`limit: 1000`, but
  the backend caps a page at 100). Fix: it searches on the server as you type, like
  `AddActivityDialog`. The `allContacts` prop and its preloading in `ActivitiesPage`, `NotesPage`,
  `NetworkPage` and `useTimelineEditing` go away.
- #107 tests must check user-visible results (ADR 0003). Do not chase lines: coverage numbers only
  point at gaps. When coverage rises, raise `GO_COVERAGE_MIN` (Makefile) or the Vitest thresholds in
  the same commit.
- A bug that a new test finds: if it is small and local, fix it in the same commit (type `fix:`,
  changelog `Fixed`). Otherwise record it under `## Questions` and leave the failing test out.
- #107 boxes that need no new commit get a reason in the audit: "REST API incl. `user_id` isolation"
  is enforced by `backend/routes/invariants_test.go`; keep it so unless the audit finds a route the
  invariants miss.
- #118 API: one huma operation `GET /api/v1/search?q=&type=&limit=&offset=`. Without `type` it
  returns three groups (`contacts`, `notes`, `activities`), each with `total` and the top `limit`
  items; with `type` it pages one group. Ranking: FTS5 `bm25`; contacts that match on a name rank
  above contacts that match on another field. Trashed rows never show. Every query is scoped to
  `user_id`, with an isolation Go test.
- #118 highlight: the API returns the snippet as segments `[{text, match}]`, never HTML. The
  frontend renders `match` segments in `<mark>`. Each contact item says which field matched
  ("Address", "CardDAV note", custom field name, ...) when it is not a name. The LIKE fallback
  (phone digits, Han/Kana/Hangul, missing FTS5) builds the same segments in Go.
- #118 index: migration 000034 recreates `contacts_fts` with the new columns and triggers (down
  restores the 000029 version). The existing `search` parameter of the contacts list and of the
  contact pickers keeps searching only names, emails and phones (FTS5 column filter), so a picker
  does not offer every contact who lives in "Berlin".
- #118 targets: a contact result opens the contact. A note result opens its contact, or the Notes
  page with the term as its search for a note without a contact. An activity result opens the
  Activities page with the term as its search. List pages read the term from `?search=`.
- #118 header search UX (steps 11-13): at least 2 characters; ~300 ms debounce; a stale response
  never replaces a newer one; loading and "no results for …" states; contacts first (up to 5), then
  notes and activities (up to 3 each), each group with a heading and its total; the last option is
  always "Show all results for …"; arrow keys move across all groups, Enter opens the highlighted
  option or, with none, the results page (until step 13: the contacts list, as today); Esc closes.
  Use MUI `Autocomplete` grouping so the list keeps its ARIA roles.
- One changelog bullet for #118 (`Added`), written in step 11 and updated in steps 12 and 13. The
  #107 test commits get no changelog entry.
- Run name for branches: `coverage-and-search`.

## Steps

- [x] 0. **Environment check** (no own commit; record in PROGRESS.md with step 1): `make precommit`
      green, whether `make test-e2e` runs. A failure that blocks every step goes under `## Blocked`.
- [x] 1. **Activity edit finds every contact (#140)** (`fix:`, `fixes #140`): server-side search in
      `EditTimelineItemDialog` as settled above; remove `allContacts`. Tests: a Vitest test that the
      dialog offers a contact the API returns for the typed term; extend `e2e/activities.spec.ts`:
      edit an activity, add a second contact by typing its name, save, see both names. Changelog
      `Fixed`.
- [x] 2. **Coverage audit (#107)** (`chore:`, `issue #107`): run `make test-coverage`, read the
      per-package and per-function numbers and the test files per area of #107. Write
      `## Coverage audit (#107)` in PROGRESS.md: per checkbox the status, the gaps (user workflows
      that no test would catch), and which of steps 3-10 covers each gap. Known low spots to check
      first: `carddav` (`BasicAuthMiddleware`, principal and address-book discovery,
      `PutAddressObject`), `services/contact_sync_service.go` (`Discover`, `resolveWellKnown`,
      `probeAddressBook`, `fetchNeededCards`, `applyPhoto`), `services/import_session.go`
      (`Confirm`), `services/monica_import_session.go` (`processAvatars`, `fail`),
      `exports/internal/app`, `services/mailer.go` (SMTP), `services/email_outbox.go`
      (`expireEmail`), `users/internal/adapters/http/oidc.go` (`oidcLogin`). If the audit shows a
      gap larger than steps 3-10 can hold, record it under `## Questions`; do not add steps.
- [x] 3. **Tests: CardDAV server discovery and auth (#107)** (`test:`, `issue #107`): through the
      HTTP handler: `.well-known/carddav` redirect, principal, address-book home set, address-book
      list; Basic auth with password, with API token, wrong password, and one user who asks for the
      address book of another user.
- [x] 4. **Tests: CardDAV server writes (#107)**: PUT create and update, `If-Match` /
      `If-None-Match` conflicts (412), DELETE (contact goes to trash and leaves the address book), a
      vCard with a photo. Check the result through GET/REPORT or the REST API.
- [x] 5. **Tests: CardDAV client sync (#107)**: discovery (well-known, probe, errors the user sees),
      remote create/update/delete reaching Meerkat and back, conflicts, photos. Use the existing
      fake-server harness of `services` (it also runs against Radicale with `MEERKAT_CARDDAV_IT=1`).
- [x] 6. **Tests: import and export (#107)**: vCard and CSV import through preview and confirm
      (duplicates, invalid rows), Monica import incl. avatars and a failing remote, contact export
      (vCard, CSV) and its content.
- [x] 7. **Tests: reminders, birthdays, notifications (#107)**: triggered scheduler runs (injected
      clock) send the right reminder and birthday emails once; SMTP delivery against an in-process
      fake SMTP server; outbox retry and expiry.
- [x] 8. **Tests: auth (#107)**: OIDC login against an `httptest` fake provider (new user, linked
      user, bad state, provider error), API tokens (scope limits, revoked token), password login
      lockout and 2FA paths not yet covered.
- [x] 9. **Tests: frontend components with logic (#107)**: Vitest and Testing Library for
      `AddContactDialog` and `ReminderDialog` (IMPROVEMENTS 2.4), and other components the audit
      names. Raise the Vitest thresholds. Remove item 2.4 from `IMPROVEMENTS.md`.
- [x] 10. **E2E: main workflows (#107)**: the main workflows the audit finds without an E2E test, at
      least import a vCard file and export contacts. Few, solid tests (ADR 0003). Final audit update
      in PROGRESS.md: every #107 box checked or with a reason.
- [x] 11. **Global search: grouped quick results (#118)** (`feat:`, `issue #118`): the search API
      settled above over the existing indexes, with highlight segments and the LIKE fallback; the
      header search box shows the groups (UX settled above; "Show all results" comes in step 13). Go
      tests: ranking, highlight, fallback, trash, isolation. E2E: type a word from a note, see the
      note under "Notes" with the word marked, open it. New use case in `docs/use-cases.yml`;
      `make openapi`; `docs/api-reference.md`; i18n in five locales; changelog `Added`.
- [ ] 12. **Search all contact fields (#118)** (`feat:`, `issue #118`): migration 000034 with the
      fields settled above; each contact result names the matched field. Go tests: one per field
      kind (plain column, JSON array, JSON objects, custom fields), the update triggers, the
      contacts list search still ignores the new fields, migration round trip. Update the changelog
      bullet.
- [ ] 13. **Search results page (#118)** (`feat:`, `fixes #118`): `/search?q=` (the term lives in
      the URL; back button works), a search field filled with the term, tabs All / Contacts / Notes
      / Activities with counts; "All" shows up to 10 per group with "Show all N" that switches tab;
      a tab pages its group. Notes and activities show date, contacts and snippet. List pages read
      `?search=`. E2E: Enter in the header search opens the page; switch to a tab; open a result.
      Update the changelog bullet.
- [ ] 14. **Finish**: check README, `docs/`, `docs/api-reference.md`, `DIFFERENCES.md` against the
      run; summary and the "For review" list in PROGRESS.md; then, as the previous run did, reduce
      PLAN.md to `## Next steps` and reset PROGRESS.md to the template, in its own commit.

## Next steps

After this run, proceed with (in order):

- https://github.com/omarkohl/meerkat-crm/issues/109 (merge contacts). Settle first: merge dialog
  and conflict UI, removal from synced CardDAV devices, undo or confirmation, API shape.
- Search follow-ups the user may want: a keyboard shortcut to focus the header search (`/` or
  `Ctrl+K`, see IMPROVEMENTS "Quick-capture command palette"), reminders in the search.
- pick further work from `IMPROVEMENTS.md` (open items only).
