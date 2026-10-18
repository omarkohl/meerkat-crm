# Progress

## Next step

Step 9: frontend component tests (#107).

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
  unreadable vCards) and `monicaimport_test.go` (avatars saved and failed ones counted, a remote
  failing mid-fetch shows a safe error and imports nothing; it takes about 6 s because of client
  retries). Bug found and fixed: confirming "add" on a row with validation errors (CSV row without
  first name, unreadable vCard) imported it; such rows are now skipped. Export already has content,
  isolation and audit tests; the `RenderCSV` gap is write-error branches only, so no export test was
  added. Go coverage 83.7%, `GO_COVERAGE_MIN` 82 to 83. `make precommit` green.
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

## Decisions

- Step 1: the E2E test cannot reproduce the 100-contact cap (few seeded contacts); the Vitest test
  covers the server search.

## Coverage audit (#107)

Percentages only point at code; each gap below is a user workflow that no test would catch. Go
function coverage of `Error()`/`Unwrap()` types, `main()` and logger set-up is left out.

- [x] **Audit coverage per area** - done by this section. Low spots confirmed: `services` (contact
      sync discovery, imports, mailer), `carddav` (server discovery and auth), `httputil`
      (`FetchImageFromURL` 18%), `monica` (`FetchContactPhoto` 0%). Frontend: 28 test files for 35
      components, 14 pages and 15 API modules.
- [x] **REST API incl. `user_id` isolation** - no new commit, with proof. `invariants_test.go`
      checks per route for every mutating route (`isolation` map, line ~30) and per model for GET
      routes (`covered` map). I checked every ID-taking GET route by hand against the module tests;
      each has a test where user B reads user A's id and gets 404: `/activities/:id`,
      `/contacts/:id/activities` (activities_test.go); `/notes/:id`, `/contacts/:id/notes`
      (notes_test.go); `/reminders/:id`, `/contacts/:id/reminders`,
      `/contacts/:id/reminder-completions` (reminders_test.go); `/contacts/:id/relationships`,
      `/contacts/:id/incoming-relationships` (relationships_test.go); `/contacts/:id`
      (contacts_test.go); `/contacts/:id/profile_picture` (photos_test.go); `/webhooks/:id`,
      `/webhooks/:id/deliveries` (webhooks_test.go); `/admin/users/:id`, `/admin/backups/:name` (a
      plain user is refused, admin tests); `/ics/:file` (tokens_test.go, token is the credential);
      `/carddav/*` (backend_test.go). List GETs without an id are scoped in the module suites
      (contacts, relationships graph, reminders upcoming, trash, sessions). Gap: nothing enforces
      the GET-per-route rule; `invariants_test.go` could list ID-taking GET routes the way it does
      mutating ones. Not a regression risk today; recorded under Questions. Low `toAppError` mappers
      (exports 0%, trash 25%, activities 27%, webhooks 27%, notes 41%, contacts 13%) are
      error-to-status mapping of rare errors; the steps that touch those modules cover the
      user-visible ones (4, 6, 7).
- [ ] **CardDAV sync** - workflows without a test:
  - Server (steps 3 and 4): a phone or desktop client follows `/.well-known/carddav` to the
    principal, the address-book home set and the list of address books; Basic auth with the account
    password, with an API token, and with a wrong password; one user asking for the address book of
    another; creating a contact with PUT and updating it with a matching `If-Match`; a stale
    `If-Match` or an existing UID with `If-None-Match` answers 412; a vCard with a photo arrives as
    the contact's photo.
  - Client (step 5): the user enters a server URL and the app finds the address book via well-known,
    via the probe, or shows the error for a wrong password, a missing address book and a non-CardDAV
    URL; a card created, changed or deleted remotely reaches Meerkat on the next sync and the
    reverse; a contact changed on both sides is resolved by the settled conflict rule; a remote card
    with a photo sets the contact photo, a broken photo URL does not fail the sync. Pointers:
    `Discover` 36%, `probeAddressBook` 33%, `fetchNeededCards` 33%, `applyPhoto` 27%,
    `SyncAllContactConnections` 0%.
- [x] **Import and export (vCard, CSV, Monica, ICS)** (done in step 6) - workflows without a test
      (step 6):
  - CSV confirm with a skipped duplicate row, an "update" row that merges into the existing contact
    (and adds the merge note), an "update" on a trashed contact that restores it, an "add" row that
    fails validation and appears under errors. Pointers: `Confirm` 44%, `MergeImportedContact` 74%.
  - vCard import with duplicates and a vCard with a photo. Pointer: `ConfirmVCF` 62%.
  - Monica import: contacts arrive with their avatar; a failing avatar download leaves the contact
    and reports a warning; a failing Monica server shows the user-safe error and ends the session.
    Pointers: `processAvatars` 0%, `fail` 0%, `FetchContactPhoto` 0%, `userSafeMonicaError` 50%.
  - Export: the CSV and vCard download contain the user's data (CSV `RenderCSV` 72%: the uncovered
    rows are listed in step 6). The E2E download comes in step 10.
  - ICS: a calendar subscription that returns an error or an unchanged feed shows the error / keeps
    its events (step 6). The ICS birthday feed has an E2E test; no further step.
- [x] **Reminders, birthdays, notifications** (done in step 7) - covered: reminder mails once per
      due date with an injected clock, mail failures, no channel, webhooks
      (`reminders/digest_test.go`, e.g. `TestDigestDoesNotMailTheSameReminderTwice`,
      `TestDigestLeavesAUserWhoseMailFailedUnmarkedAndStillMailsTheOthers`); the job lock
      (`TestSendRemindersWithRateLimit_RateLimited`, `_AllowsAfterInterval`); outbox retry, backoff,
      claim and expiry (`email_outbox_test.go`). New in step 7: SMTP delivery and its failures
      against a fake server, Resend through a swapped transport, and an e-mail that expires after
      its claim (`expireEmail`). Open, see Questions: the birthday mail is not once per day.
- [x] **Auth (password, OIDC, API tokens)** (done in step 8) - the audit listed workflows that were
      already tested by name: OIDC new user, link by verified e-mail, bad state or nonce, provider
      error, no account without auto-provisioning (`users/oidc_test.go`:
      `TestASuccessfulCallbackProvisionsTheAccountStartsASessionAndRedirectsHome`,
      `TestAVerifiedEmailLinksTheIdentity...`,
      `TestACallbackThatCannotBeTrustedEndsInTheLoginPage...`, `TestTheNonceAndTheTokenAreChecked`,
      `TestWithoutAutoProvisioningAStrangerIsToldThereIsNoAccount`); password and second-factor
      lockout (`users/login_test.go`); login with a recovery code that works once and new recovery
      codes that invalidate the old (`TestAnAccountWithTwoFactorAsksFor...`,
      `TestDisablingWorksWithACodeOrAOneTimeRecoveryCode`); API token checks in the middleware
      (`middleware/auth_apitoken_test.go`). New: `routes/api_token_flow_test.go` runs the whole path
      through the real router and migrations: register, log in, create a token, use it for data, see
      it refused for password change, 2FA setup, sessions and admin routes, revoke it, and see it
      refused. No E2E for tokens (Go covers it). Open: the `oidcLogin` error branches (state or
      nonce generation failing, answered 500) are reachable only through the `app.Identity` port
      (`NewToken` failing), which `users.New` builds itself, so a test needs a fake `Identity` in an
      `app`-level test; not added. Password reset mail with a failing SMTP server: the reset mail
      goes through the outbox (`TestQueueEmailWritesRowBeforeSending`,
      `TestProcessEmailOutboxBackoffThenDead`, `TestExpiredEmailIsNeverSentAndLosesItsBody`) and
      SMTP failures through `smtp_delivery_test.go`; no test joins the two.
- [ ] **E2E: main user workflows** - existing: auth, contacts, timeline, activities, reminders,
      relationships, trash, 2FA, sessions, backups, ICS feed, isolation, dashboard, thumbnails,
      security headers, users banner. Missing: import vCard/CSV, export contacts (step 10);
      register, CardDAV/calendar settings, network graph, notes page stay out with a reason:
      settings pages are thin over API calls the Go tests cover, the graph is visual.
- [ ] **Frontend: components with logic** - Vitest lines 26.9%. Step 9 takes `AddContactDialog`
      (0.9%), `ReminderDialog`, `ReminderList`, `EditableField`, `MultiValueField`, `useDebounce`,
      `useReminderManagement`. The remaining 0% components and pages are under Questions with a
      reason. API modules at 0% are covered by E2E, not unit tests.

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

## Blocked
