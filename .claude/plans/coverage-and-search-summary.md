# Coverage and search run: summary

Run `coverage-and-search`, 2026-10-18. Thirteen commits, all local (nothing pushed). Goals: fix the
contact search when an activity is edited (#140), close the regression-test gaps of #107, add a
global search over contacts, notes and activities (#118).

Commits `470045b14271` and `5b4c460cf0c8` in the stack are not from this run (made outside the
session); the run did not touch them.

## What the run delivered

- **#140** Editing an activity searches all contacts on the server as you type; the preloaded list
  (capped at 100 contacts) is gone.
- **#107** (steps 2 to 10): a coverage audit and tests per area. Go total coverage 81.9% to 84.2%
  (`GO_COVERAGE_MIN` 71 to 83), Vitest lines 26.9% to 31.8% (thresholds raised to 30/28/25/31). The
  tests found and fixed six bugs: CardDAV ETags with second granularity (a stale `If-Match` passed),
  ignored `If-None-Match`, `If-Match` on a missing contact created it, 500 instead of 404 (another
  user's address book, a missing contact), import "add" of invalid rows (CSV, vCard, Monica), and a
  vCard decode error that shifted every later card (the last confirm panicked).
- **#118** (steps 11 to 13): `GET /api/v1/search` (one huma operation, new module `globalsearch`),
  the header search with groups, highlight and field labels, migration 34 (every contact text field
  in the FTS index), and a results page `/search?q=` with a tab per group and paging.

## For review (decide or look at)

- **Search UI choices:** the header asks for 5 contacts and shows 3 notes and 3 activities (the API
  has one `limit`); "Show all results" and Enter open the results page; the contact list with a term
  is reached from the Contacts tab ("Show in the contact list"); a custom field is found by its
  value, not its name; archived contacts are found; a note of a trashed contact is hidden.
- **API:** `type` returns only the requested group; `offset` without `type` is a 400; `q` is at most
  200 characters; a contact result names `matched_field` (and `matched_field_name` for a custom
  field); an activity result lists its contacts.
- **Migration 34** recreates `contacts_fts` and refills it from the contacts; the down migration
  restores the migration-29 index.
- **Behaviour changes from fixes:** CardDAV conditional writes now answer 412 as the standard says;
  imports skip rows with errors even when asked to add them.
- **Not from the plan:** the Vitest timeout of two older test files (`EditTimelineItemDialog`,
  `AddContactDialog`) was raised to 20 s because they timed out under the coverage run.

## Open questions

- The plan and audit expect CardDAV Basic auth with an API token. The server accepts the account
  password only (`docs/carddav.md`); a token as password is refused, and the test pins that. If
  tokens should work, it is a feature, not a test.
- `invariants_test.go` enforces per-route isolation for mutating routes only; GET routes with an id
  rely on module tests (checked by hand in the audit). A per-route map for ID-taking GETs would keep
  it so.
- Not in step 9, with the reason: `AddNoteDialog`, `AddRelationshipDialog`,
  `ProfilePictureUploadDialog` are thin forms over tested API calls and covered by the timeline,
  relationships and thumbnails E2E tests; `ContactHeader`, `ContactTimeline` are display only
  (timeline has E2E); `CustomFieldsSettings`, `ContactFieldSettings`, `EditableArrayField` are
  settings/editing glue with no branching beyond rendering a list. Revisit if the user wants more
  unit tests.
- Frontend pages (`ContactDetailPage`, `ContactsPage`, `NotesPage`, `ActivitiesPage`) and most hooks
  stay near 0% unit coverage after step 9; E2E covers the main paths. The audit proposes no extra
  step; the user may want one for `ContactDetailPage`.
- Contact search in the activity dialogs is duplicated in `AddActivityDialog` and
  `EditTimelineItemDialog`; a shared `useContactSearch` hook would remove it.
- The edit `IconButton` at `ActivitiesPage.tsx` (~line 284) has no accessible name.
- The CardDAV test router copies `registerCardDAVRoutes` (`backend/routes/routes.go:150`) without
  `CardDAVRateLimitMiddleware`; a shared register function would stop drift.
- go-webdav does not pass `If-Match` on DELETE to the backend, so a stale DELETE still succeeds.
- The birthday mail has no sent mark: every digest run on the birthday sends it again. Only the
  one-hour job lock (`SendRemindersWithRateLimit`) limits repeats, and the startup run (`main.go`)
  fires it too, so a restart more than an hour after the daily run mails the birthdays a second time
  (probe: two runs two hours apart sent 2 mails). A fix needs the last-sent date per user (a
  migration) or a design choice, so no test was added.
- `contacts.add.requiredFields` says "First name and last name are required" but only the first name
  is checked in `AddContactDialog`; the text (five locales) or the check is wrong.
- `AddContactDialog.tsx` (~line 205) compares the birthday at midnight with the current time
  (`nextBirthday < today`), so a contact added on its birthday gets its first reminder next year.
  Not fixed.
- `AddContactDialog.tsx` (~line 159) sends the first name untrimmed (the check uses the trimmed
  value). Not fixed.
- `ImportContactsDialog.tsx` (~line 341): the drop zone is a clickable `Box` with no role or
  `tabIndex`, so keyboard users cannot open the file chooser.
- The search fallback (LIKE, used for digits, Han/Kana/Hangul text and a missing index) lower-cases
  with SQLite's `LOWER`, which is ASCII only, so a mixed-case non-ASCII term (for example `Ü`) can
  miss in the fallback; the index path folds correctly.
- `fetchNeededCards` (CardDAV client, multiget after a sync-collection) only runs against Radicale
  (`make test-carddav`); no test in the default suite reaches it.
- The `oidcLogin` 500 branches (state or nonce generation failing) are reachable only through the
  `app.Identity` port, which `users.New` builds itself; a test needs a fake `Identity` in an
  `app`-level test.
- The Notes and Activities pages filter by `?search=` (a result of the header search opens them that
  way), but no E2E test opens a note without a contact from the results page and checks the filtered
  Notes list.
- `useReminderManagement` (94 lines) has no unit test: it only wraps API calls and dialog state, and
  `e2e/reminders.spec.ts` drives it through the page.

## Final #107 audit (copy this into the issue)

Final state after steps 3-10. Go total coverage 81.9% to 84.2% (`GO_COVERAGE_MIN` 71 to 83), Vitest
lines 26.9% to 31.8% (thresholds 25/23/19/25 to 30/28/25/31). Numbers only point at gaps; the tests
check user-visible results. Bugs the new tests found and fixed: CardDAV ETags (second granularity),
ignored `If-None-Match`, `If-Match` on a missing contact, 500 instead of 404 (address book, missing
contact), import "add" on invalid rows (CSV, vCard, Monica), a vCard decode error shifting every
later card (panic on confirm).

- [x] **Audit coverage per area** - this section. Low spots found: `carddav`, `services` (contact
      sync discovery, imports, mailer), `monica`, frontend dialogs, `httputil`. `httputil`:
      `SameServer` is fully covered; `FetchImageFromURL` has one test, which checks that internal
      targets are refused (17.6% of the function); its success path needs a loopback server, which
      the SSRF guard it tests blocks, so it stays open.
- [x] **REST API incl. `user_id` isolation** - no new commit. `routes/invariants_test.go` checks
      every mutating route (per route) and every model with a `user_id` (per model). Every ID-taking
      GET route has a module test where user B gets 404 on user A's id (checked by hand in the
      audit). Open: nothing enforces that rule per GET route (Questions).
- [x] **CardDAV sync** - steps 3 and 4 (server): discovery via well-known, principal, home set,
      Basic auth (password, lockout, a token as password is refused), another user's address book
      (404), PUT create and update, `If-Match` and `If-None-Match` conflicts (412), DELETE to the
      trash, photos (`carddav/server_test.go`). Step 5 (client): discovery from a server or
      address-book address, the errors the user sees, no password sent to another host, photos,
      scheduled sync over several connections (`services/contact_sync_discovery_test.go`); create,
      update, delete and conflicts were covered. Open: `fetchNeededCards` (multiget) only runs
      against Radicale (`make test-carddav`).
- [x] **Import and export (vCard, CSV, Monica, ICS)** - step 6: CSV update of a duplicate with a
      merge note, update without a duplicate, vCard photo and re-import, unreadable vCards, invalid
      rows, Monica avatars and a failing remote (`imports_test.go`, `monicaimport_test.go`). Already
      covered: restoring a trashed contact on re-import
      (`TestVCFReimport_AfterDelete_RestoresContact`), ICS errors and unchanged feeds
      (`calendar_sync_service_test.go`), export content, isolation and audit (`exports_test.go`).
      Step 10: E2E import of a vCard and a CSV, E2E export as vCard and CSV.
- [x] **Reminders, birthdays, notifications** - step 7: SMTP delivery and its failures against a
      fake server (`smtp_delivery_test.go`), Resend through a swapped transport, an e-mail that
      expires between claim and send. Already covered: reminder and birthday digest, job lock,
      outbox retry and expiry (`reminders/digest_test.go`, `email_outbox_test.go`). Open: the
      birthday mail has no sent mark and goes out again on every digest run that day (Questions).
- [x] **Auth (password, OIDC, API tokens)** - step 8: an API token through the real router (use,
      refused for password change, 2FA, sessions, admin, revoked). Already covered: OIDC (new user,
      link, bad state or nonce, provider error, no account), password and second-factor lockout,
      recovery codes, token middleware. Open: the `oidcLogin` 500 branches need a fake `Identity`.
- [x] **E2E: main user workflows** - step 10 adds `e2e/import-export.spec.ts` (import vCard, import
      CSV, export vCard and CSV). Existing: auth, contacts, timeline, activities, reminders,
      relationships, trash, 2FA, sessions, backups, ICS feed, isolation, dashboard, thumbnails,
      security headers, users banner. No E2E, with a reason: registration, CardDAV and calendar
      settings, API tokens (Go tests cover the API paths; the pages are thin), network graph
      (visual), notes page (the same timeline and note dialogs as the contact page, which has E2E
      tests).
- [x] **Frontend: components with logic** - step 9: `AddContactDialog`, `ReminderDialog`,
      `ReminderList`, `MultiValueField`, `EditableField`, `useDebouncedValue`, and
      `EditTimelineItemDialog` (step 1). Not tested, with reasons under Questions:
      `useReminderManagement`, `AddNoteDialog`, `AddRelationshipDialog`,
      `ProfilePictureUploadDialog`, `ContactHeader`, `ContactTimeline`, the settings components, and
      the large pages (`ContactDetailPage`, `ContactsPage`, `NotesPage`, `ActivitiesPage`), which
      E2E drives.
