# Progress

## Next step

Step 13: search results page (#118).

## Stack

<!-- step: branch, PR number (base) -->

Local run: no branches, no PRs.

## Log

- Steps 0 and 1: `make precommit` runs end to end in this environment, including `make test-e2e`
  (Playwright works without a workaround). `EditTimelineItemDialog` now searches contacts on the
  server (300 ms debounce, `limit: 100`, like `AddActivityDialog`); `allContacts` is gone from the
  dialog, `useTimelineEditing`, `ActivitiesPage`, `NetworkPage`, `ContactDetailPage`, `NotesPage`.
  Added Vitest test and an E2E test; the timeline edit button got an accessible name.
- Step 2: `make test-coverage` is green. Go total 81.9% (`GO_COVERAGE_MIN` 71); Vitest lines 26.9%
  (thresholds 25/23/19/25). Numbers from `backend/coverage.out` and
  `frontend/coverage/coverage-summary.json`. The audit is in `## Coverage audit (#107)`; no code
  changed.
- Step 3: `backend/carddav/server_test.go` drives the handler with Basic auth: well-known redirect,
  principal, home set, address-book list, login by username or e-mail, refusals (none, wrong
  password, unknown user, a real API token as password), lockout (429 with `Retry-After`; the global
  limiter is reset after each test), another user's address book. Found and fixed: that request
  answered 500, now 404 (`fix:`, changelog `Fixed`). `BasicAuthMiddleware` 16% to 100%.
  `GO_COVERAGE_MIN` 71 to 82 (total 82.3%, a margin of 0.3 points). `make precommit` green.
- Step 4: `server_test.go` now covers PUT create and update (ETag changes), `If-Match` and
  `If-None-Match` conflicts (412), a missing contact (404), DELETE (leaves the address book, lands
  in the trash), and a vCard with a photo. Four bugs fixed with it: ETags used whole seconds, so a
  stale `If-Match` passed after two saves in one second (now nanoseconds); `If-None-Match` was
  ignored; `If-Match` on a missing contact created it; GET/DELETE of a missing contact answered 500
  (now 404). `make precommit` green.
- Step 5: the existing fake-server scenarios already cover remote create/update/delete, conflicts
  and token mode, so `contact_sync_discovery_test.go` adds what was missing: discovery from a server
  address (the root answers 404, so it must follow `/.well-known/carddav`; the test counts the hit;
  same-host success path of `resolveWellKnown` covered), from an address-book address, and from an
  address-book address on a server without principals (`probeAddressBook`); the errors the user sees
  (wrong password, not CardDAV, unknown address, bad scheme; the private-address case lives in
  `private_urls_test.go`); no password sent to another host via well-known; photos pulled (embedded;
  an address the SSRF guard blocks imports the contact anyway) and pushed; the scheduled sync over
  several connections (a failing one in the middle, a disabled one; checked to fail when the loop
  stops at the first error). The in-process remote now has a photo directory. No bug found. Go
  coverage 82.9%, `GO_COVERAGE_MIN` stays 82. `fetchNeededCards` (multiget, Radicale only,
  `make test-carddav`) stays open. `make precommit` green.
- Step 6: new tests in `imports_test.go` (CSV update of a duplicate with its merge note, update
  without a duplicate reported as an error, vCard with an embedded photo and a re-import merged,
  rows and cards that cannot be imported, a decode error between cards) and `monicaimport_test.go`
  (avatars saved and failed ones counted, a remote failing mid-fetch with 429 and `Retry-After: 0`
  shows a safe error and imports nothing). Bugs found and fixed: confirming "add" on a row with
  validation errors (CSV row without first name, unreadable vCard, Monica contact without a name)
  imported it, and a vCard decode error shifted every later card by one row (the last confirm
  panicked); invalid rows are now skipped and the parser keeps rows aligned. Already covered, not
  repeated: re-import of a trashed contact restores it
  (`TestVCFReimport_AfterDelete_RestoresContact`), ICS errors and unchanged feeds
  (`calendar_sync_service_test.go`), export content (`exports_test.go`). Go coverage 83.7%,
  `GO_COVERAGE_MIN` 82 to 83. `make precommit` green.
- Step 7: the digest tests (`reminders/digest_test.go`) already cover reminder and birthday mails
  with an injected clock for reminders (a birthday mail is not limited to once a day, see
  Questions), mail failures, no channel and webhooks, and `email_outbox_test.go` the retry and batch
  expiry, so no test was repeated. New: `smtp_delivery_test.go` runs `SendEmail` against an
  in-process fake SMTP server (message, recipient and body arrive; wrong password, refused
  recipient, untrusted implicit-TLS certificate and untrusted STARTTLS all reach the caller and
  deliver nothing; no sleeps), and an outbox test for an e-mail that expires between the claim and
  the send (`expireEmail`), and Resend delivery through a swapped `http.DefaultTransport`. No bug
  fixed; one found (see Questions). Go coverage 84.1%; `GO_COVERAGE_MIN` stays 83 (margin 1.1).
  `make precommit` green.
- Step 8: most of the workflows the audit listed for auth were already tested (see the audit box for
  the names), so the one new test is `routes/api_token_flow_test.go`: a token created by a logged-in
  user writes and reads data through the real router, is refused for password change (401, as the
  sessions test documents), 2FA setup, sessions and admin routes (403), does not change the
  password, and is refused with 401 once revoked. No bug found. Go coverage 84.2%; `GO_COVERAGE_MIN`
  stays 83. `make precommit` green.
- Step 9: new Vitest tests: `ReminderDialog` (message required, trimmed save with today's date, a
  recurrence moves the date of a new reminder and offers rescheduling from completion, editing keeps
  the date, a failed save shows the error and stays open), `AddContactDialog` (first name required,
  empty rows dropped, success message, invalid birthday refused, the birthday reminder switch
  appears only with a birthday and the reminder is due on the next birthday, this year or next,
  circles without duplicates, a server error keeps the dialog open), `ReminderList` (empty state,
  recurrence, e-mail and flexible chips, complete and edit on the clicked card, delete only after
  confirming), `MultiValueField` (add, type, delete rows, custom type label), `EditableField`
  (display value, suffix, formatted value with raw value for editing, empty mark, draft, save,
  cancel, validation error only while editing) and `useDebouncedValue` (fake timers). They use the
  injected clock via `vi.setSystemTime` (Date only) and no sleeps. No bug fixed; two found (see
  Questions). Vitest coverage 31.1% statements, 28.4% branches, 25.8% functions, 31.8% lines;
  thresholds raised to 30/28/25/31 (margins 1.1, 0.4, 0.8, 0.8 points). IMPROVEMENTS item 2.4
  removed. `make precommit` green.

- Step 10: `e2e/import-export.spec.ts`: import a vCard file and a CSV file through the dialog (file
  chooser, review, import, contact found in the list by its name) and export all contacts as vCard
  and CSV from Settings > Data (download content checked). Ran `make test-e2e` three times. No bug
  found. This closes #107; the final audit below is meant for the issue. `make precommit` green.
- Step 11: new module `backend/globalsearch` (domain: highlight and excerpt as segments; app: one
  use case over a port; persistence: FTS5 with bm25 and name-first ranking, LIKE for digits, CJK and
  a missing index; http: `GET /search`), wired in `routes`, with the three depguard layering rules
  like the other modules and a read-isolation entry in `invariants_test.go`. `make openapi`,
  `docs/api-reference.md`, `docs/data-model.md`, use case 022-US1, five locales, changelog.
  Frontend: the header search moved from `App.tsx` into `components/HeaderSearch.tsx` (groups,
  debounce 300 ms, 2 characters, stale-response guard, loading and no-results rows, "Show all
  results" last); Vitest for it; E2E `search.spec.ts` (a word from a note shows under Notes with the
  word marked and opens the contact). Mutation checks: dropping the name-first order or the bm25
  order fails Go tests. `make precommit` green.
- Step 12: migration 34 recreates `contacts_fts` with the free-text fields (middle name,
  organization, department, job title, role, how we met, food, work and contact information, the
  CardDAV note, addresses and the legacy address, circles, custom field values) and fills it from
  the existing contacts; the down migration restores the 29 index and refills it. Triggers cover
  insert, update of any indexed column and delete; trash and restore need nothing because the index
  holds deleted rows too. The contact list keeps searching names, e-mail addresses and phone numbers
  through a column filter (`search.ContactBasics`). Each contact result names the matched field
  (`matched_field`, `matched_field_name` for a custom field); the LIKE fallback covers the same
  fields. Tests: migration round trip, triggers and JSON kinds in `database`, every field kind, the
  fallback, edit/trash/restore, isolation and the list search in `globalsearch` (dropping the column
  filter fails a test). `make precommit` green.

## Decisions

- Step 12: a custom field is found by its value, not its name; the legacy `address` is reported as
  `address` unless it equals a structured address; the field label in the header uses the existing
  `contacts.*` labels, plus `search.noteField` ("CardDAV note").

- Step 11: `type` returns only the requested group (the others are omitted); the UI asks for
  `limit=5` and shows 3 notes and 3 activities by slicing, since the API has one `limit`.
- Step 11: Enter with no option highlighted still opens the contacts list for the term (until step
  13); note and activity results open the contact, `/notes?search=` or `/activities?search=` as
  settled, but those pages read `?search=` only in step 13.
- Step 11: archived contacts are found; a note is hidden when its contact is trashed, a note without
  a contact is found.
- Step 11: `offset` without `type` is refused with 400 (only one group can be paged); `q` is at most
  200 characters. module).

- Step 1: the E2E test cannot reproduce the 100-contact cap (few seeded contacts); the Vitest test
  covers the server search.

## Coverage audit (#107)

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

## For review

## Questions

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
- `useReminderManagement` (94 lines) wraps the reminder API calls and the dialog state of
  `ContactDetailPage`; it has no branching of its own beyond error reporting, and
  `e2e/reminders.spec.ts` drives it through the page. A hook test would mostly mock the API. Not
  added.
- `AddContactDialog.tsx` (~line 205) compares the birthday at midnight with the current time
  (`nextBirthday < today`), so a contact added on its birthday gets its first reminder next year.
  Not fixed.
- `AddContactDialog.tsx` (~line 159) sends the first name untrimmed (the check uses the trimmed
  value). Not fixed.
- The Vitest thresholds are rounded down with a small margin; the lines margin is 0.8 points.
- `ImportContactsDialog.tsx` (~line 341): the drop zone is a clickable `Box` with no role or
  `tabIndex`, so keyboard users cannot open the file chooser.
- Known gap until step 13: the header's "Show all results for …" option (and Enter without a
  highlighted option) opens the contacts list filtered by the term, which shows only contacts, while
  the label promises all results. Settled in PLAN.md for step 11; step 13 adds the results page.
- The search fallback (LIKE, used for digits, Han/Kana/Hangul text and a missing index) lower-cases
  with SQLite's `LOWER`, which is ASCII only, so a mixed-case non-ASCII term (for example `Ü`) can
  miss in the fallback; the index path folds correctly.

## Blocked
