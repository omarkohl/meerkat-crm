# Progress

## Next step

Step 6: import and export tests (#107).

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
- [ ] **Import and export (vCard, CSV, Monica, ICS)** - workflows without a test (step 6):
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
- [ ] **Reminders, birthdays, notifications** - workflows without a test (step 7): a reminder and a
      birthday mail go out once per due date with an injected clock; an e-mail goes through a real
      SMTP conversation (an in-process fake server) with TLS/auth failures; a Resend request with a
      failing API answers with a retry; a rate-limited run sends the rest next time
      (`SendRemindersWithRateLimit`). The outbox retry, backoff, claim and expiry are well tested.
      `expireEmail` (0%) is not a gap: the workflow "an expired email is never sent" is covered by
      `TestExpiredEmailIsNeverSentAndLosesItsBody` through the batch `expireEmails`; `expireEmail`
      runs only when a row expires between that batch and the claim, a race the user cannot see.
- [ ] **Auth (password, OIDC, API tokens)** - workflows without a test (step 8): OIDC login creates
      a new user, links an existing one by e-mail, refuses a bad `state` and a provider error, and
      refuses a user without an account when sign-up is off (`oidcLogin` 46%); an API token
      authenticates a request, is refused on admin endpoints, and stops working after revoke (E2E is
      not planned; Go covers it); a user with 2FA who loses the authenticator signs in with a
      recovery code, and regenerating the codes invalidates the old ones (`replaceRecoveryCodes`
      73%); password reset mail with the SMTP failing.
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

## Blocked
