# Progress

## Next step

13g (users, sessions, TOTP, OIDC, password reset, same pattern).

## Stack

<!-- step: branch, PR number (base) -->

## Log

- Step 0: `make precommit` green. `make test-e2e` runs (54 passed, no workaround needed).
- Step 1: `docs/adr/` with README index and ADRs 0001-0008 (record decisions, machine checks,
  testing strategy, SQLite/WAL, user_id isolation, migrations, AppError, five locales); linked from
  `architecture.md`.
- Step 2: committed `AGENTS.md` (index, hard limits, failure modes, bad/good examples, jj and push
  rules, issue footers); `CLAUDE.md` is only `@AGENTS.md` and no longer ignored. `make precommit`
  green.
- Step 3: targets declare `install` as prerequisite; added `test-coverage`, `arch` (placeholder),
  `seed` (`scripts/seed.mjs`, needs `make dev` running; tested against a temp backend); `test-e2e`
  is in `precommit`; Checks in AUTONOMOUS-RUNS.md and run-one-test table in testing.md updated.
  `make precommit` green incl. E2E (54 passed).
- Step 4: Makefile split into `install-go`/`install-frontend`, `lint-go`/`lint-frontend`,
  `test-go`/`test-frontend`, `test-coverage-go`/`-frontend`, `test-carddav`, `test-e2e-docker`,
  `vuln-go`, `audit-frontend`, `fuzz`. Lint, unit, e2e, security and fuzz workflows call them (docs,
  docker-build, docker-publish unchanged: Jekyll and image build, not make). actionlint v1.7.7
  clean; Go version in fuzz.yml and the CardDAV job of unit-tests.yml raised 1.26 -> 1.27 (needed:
  `backend/go.mod` requires 1.27.0); CI itself not run locally. `make precommit` green.
- Step 5: the full reformat (`04bed7d6` "style: format all files with gofmt and Prettier") and
  `format-check` on all files (no `BASE` logic) were already in the history below this run
  (`4eb6e3b1`); `make format` changes nothing. So no new reformat commit; this commit only adds
  `.git-blame-ignore-revs` for `04bed7d6` and a note in contributing.md. `make precommit` green.
- Step 6: enabled `noUncheckedIndexedAccess`, `noImplicitOverride`,
  `noPropertyAccessFromIndexSignature` in `frontend/tsconfig.json`; fixed 86 errors (bracket access
  for index signatures incl. `process.env['REACT_APP_...']`, `override` on class members,
  destructuring with defaults for regex/split parts, `.slice(0, 10)` instead of `.split('T')[0]` in
  four dialogs, `?.` in tests). No non-null assertions. The three flags are also in
  `tsconfig.node.json` (E2E and Playwright config) and `make typecheck` now checks that project too.
  `make precommit` green incl. E2E.
- Step 7: ESLint 9 flat config `frontend/eslint.config.mjs` (`typescript-eslint` strictTypeChecked,
  `jsx-a11y` recommended, `eslint-comments` recommended +
  `require-description`/`no-unlimited-disable`, `react-hooks` rules-of-hooks/exhaustive-deps,
  `reportUnusedDisableDirectives: error`; all errors); `--fix` applied (mostly braces for
  `no-confusing-void-expression`); 4 unused catch/param variables, 2 undescribed disables fixed by
  hand; stale file-wide disable in `service-worker.ts` removed (rule was not active). 529 findings
  remain, bulk-suppressed (counts per file and rule) in `frontend/eslint-suppressions.json`, reasons
  per rule class in `docs/development/frontend.md`. Only `src/**/*.{ts,tsx}` is linted (e2e comes
  with step 16). Checked: a new floating promise in `src/ContactsPage.tsx` fails; fixing one makes
  `make lint-frontend` fail until `--prune-suppressions` (exit 2). Each rule shown to fail on a
  violation file (rules-of-hooks, exhaustive-deps, no-floating-promises, no-non-null-assertion,
  eslint-comments, jsx-a11y/alt-text, restrict-template-expressions). Build and tests green.
- Step 8: Vitest `coverage.thresholds` (statements 28, branches 80, functions 33, lines 28; current
  28.09/80.51/33.76/28.09) and `scripts/check-go-coverage.mjs` (test first, red then green) with
  `GO_COVERAGE_MIN=62` (current 62.9) called by `make test-coverage-go`. `precommit` runs
  `test-coverage` instead of `test`; CI already calls the coverage targets, so thresholds apply
  there. Shown to fail with a too-high threshold for both. Script tests moved from `i18n-check` to
  `make test-scripts` (in `test`, `precommit` and the lint workflow's locales job); the script
  checks its main-module test with `pathToFileURL` and rejects a blank threshold (exit 2).
  `make precommit` green.
- Step 9: ADR 0009; `backend/clock` (Clock, Real, Fake, `FAKE_NOW` clock; tests first);
  `middleware.ClockMiddleware` + `RequestNow`; scheduler tasks take `clk.Now()` once per job; all 84
  `time.Now()` outside main/middleware/clock/metrics replaced (services take `now time.Time`, import
  session managers hold a clock set by `controllers.SetImportClock`, GORM `NowFunc`, JWT time func);
  `forbidigo` bans `time.Now` with the ADR link (shown to fail on a violation file; two reasoned
  `nolint`: SMTP deadline, health duration); `FAKE_NOW` config + validation + warning; frontend
  `src/utils/now.ts` and an ESLint ban on `new Date()`/`Date.now()` elsewhere in `src` (shown to
  fail). Docs: backend.md, getting-started.md. One commit. Review fixes: job-like work (outbox,
  webhooks, reminders, calendar and contact sync) takes the `clock.Clock` and reads it where it
  stamps something (after slow I/O, lock release at the end); TOTP always uses the real time; purge
  jobs are skipped while `FAKE_NOW` is set; `forbidigo` runs with `analyze-types` (an aliased `time`
  import is caught; without it the alias passed); Monica import reads the clock once per import;
  `Date()` without `new` is banned too; tests for the manager clocks and for retry scheduling after
  a slow send; `FAKE_NOW` in deployment.md and `.env.example` (test only). `make precommit` green.
- Step 10: ADR 0010; Go `depguard` rules (models, services, controllers) in `.golangci.yml` with no
  exemptions (one unused gin router in a services test removed); frontend `dependency-cruiser` rules
  (no-circular, no-unresolvable, pages-use-api-through-hooks, api-client-is-private-to-api) with the
  30 existing pairs in `.dependency-cruiser-known-violations.json` and `scripts/arch-frontend.mjs`
  (new violations fail, fixed-but-still-listed ones fail too; test first). Two cycles fixed
  (`types/entities.ts`). Every rule shown to fail on a violation; the `src/api/index.ts` barrel no
  longer re-exports `client.ts` and a rule keeps it so. `make arch` runs both (already in
  `precommit`), new `arch` job in lint.yml. `make precommit` green.
- Step 11: spike passed all checks, no blocker. ADR 0011; `backend/api` (huma v2.39.1 mounted on Gin
  groups via an own adapter with per-operation Gin middleware; AppError error format incl. 422 ->
  400; `ValidateBody` for the six validators, tested on all six; request clock timestamp);
  `cmd/openapi`; `make openapi` / `openapi-check` (in precommit and a CI job); `openapi-typescript`
  types in `frontend/src/types/generated/api.ts`; `GET /auth/oidc/config` converted and
  `useOIDCConfig` uses the generated type. `make precommit` green.
- Step 11 follow-ups (squashed into step 11): error logging, request bodies as with `ShouldBindJSON`
  (unknown fields ignored, required from `validate` tags, JSON detail keys), 413/415,
  `MaxBodyBytes`, byte differences documented, `go mod tidy`, goimports, unconditional registration
  documented.
- Step 12a: ADR 0012 (module layout); `backend/contacts` with `internal/domain`, `internal/app` (use
  cases `ArchiveContact`, `UnarchiveContact`, `DeleteContact`, `ListCircles`; ports `Contacts`,
  `Store`/`Tx`, `Events`), `internal/adapters/{persistence,http,webhooks}`; huma operations for
  `POST /contacts/{id}/archive`, `/unarchive`, `DELETE /contacts/{id}`, `GET /contacts/circles` with
  a typed `ContactResponse` (same JSON names as the GORM model); `api.RegisterOn` puts operations on
  the authenticated Gin group, `api.UserID/Now/Clock` read the request; composition in `main.go`
  (`routes.Modules`); depguard rules `contacts-domain-stays-pure`,
  `contacts-use-cases-depend-on-ports`, `contacts-repositories-stay-in-persistence` (each shown to
  fail); old Gin handlers and their tests removed, replaced by module tests on real SQLite (response
  keys, reminders deleted, trash, photos kept, isolation 404 for every operation, 401 without a
  user); frontend uses the generated `ContactResponse` for archive/unarchive. `make precommit`
  green.
- Step 12b: huma operations `GET /contacts/{id}` (`fields`), `POST /contacts` (201),
  `PUT /contacts/{id}`; domain `SyncPrimary` (primary email/phone/address) and `Replace`; use cases
  `ShowContact`, `CreateContact`, `UpdateContact`; ports `Contacts.Show`, `Tx.Create/Update`, events
  `ContactCreated/Updated`; `ContactBody` validated by `api.ValidateBody` (replaces
  `models.ContactInput`); old Gin handlers, routes and tests removed; module tests on real SQLite
  (sync, validation, webhooks, isolation, fields). `make openapi` regenerated. Go coverage now uses
  `-coverpkg=./...` (module tests in `contacts/` did not count for `internal/`, total fell to 61.6%
  < 62); 71.2% now, `GO_COVERAGE_MIN` 62 -> 71. `make precommit` green.
- Step 12c: huma operations `GET /contacts` (search/FTS, circle, sort, order, paging, archived
  filters, `fields`, `includes`), `/contacts/random`, `/contacts/birthdays`; use cases
  `ListContacts` (normalises paging, order), `RandomContacts`, `UpcomingBirthdays`; persistence
  ports `List`, `Random`, `UpcomingBirthdays` (calls `services.GetUpcomingBirthdays`, which
  reminders still use). `controllers/contact_controller.go` and `models.ContactResponse` deleted;
  the controller tests of the list now mount the module (`mountContacts`). Query values are strings
  parsed leniently, so bad `page`/`limit` still fall back to defaults. Frontend: `Contact`,
  `ContactValue`, `ContactAddress`, `Birthday`, `ContactsResponse` and the create/update input are
  derived from the generated types. Fixed `api.relaxRequestBody`, which dropped the value type of
  map properties (`custom_fields` became `Record<string, never>`); test first. depguard rules were
  already in place from 12a. `make precommit` green.
- Step 13a: `backend/notes` (domain `Note.Edit`, use cases, ports `Notes`/`Store`/`Events`,
  persistence/http/webhooks adapters) with huma operations for all seven note endpoints
  (`/contacts/{id}/notes` GET/POST, `/notes` GET/POST, `/notes/{id}` GET/PUT/DELETE); webhooks
  `note.created/updated/deleted` as before; depguard rules `notes-*` (shown to fail);
  `routes.NewModules` builds all modules (main, openapi, route tests). Old controller, its tests and
  `models.NoteInput` removed; 12 module tests on real SQLite incl. isolation and 401. Contacts'
  embedded response types renamed `ContactNote/Activity/Reminder` (schema names collide). Frontend
  `Note`, `NotesResponse` and request bodies from the generated types; create/update now typed as
  the `{message, note}` they always returned. `make precommit` green.
- Step 13b: `backend/activities` (domain `Activity.Edit`, use cases, ports `Activities`/`Store`/
  `Events`, persistence/http/webhooks adapters) with huma operations for the six activity endpoints
  plus `/contacts/{id}/activities`; webhooks `activity.created/updated/deleted`; depguard rules
  `activities-*` (shown to fail); `routes.NewModules` extended. Old controller and its tests and
  `models.ActivityInput` removed (shared test helpers moved to `controllers/helpers_test.go`); 10
  module tests on real SQLite incl. isolation, search, contact links and webhook payloads. Frontend
  `Activity`, `ActivityContact`, responses and request bodies derive from the generated types; the
  dead `*ActivityResponse` types are gone. `make precommit` green.
- Step 13c: `backend/reminders` (domain `Reminder.Edit/Complete/Record/NextOccurrence`, recurrence
  maths moved from services with its tests; use cases; ports `Reminders`/`Store`; persistence and
  http adapters) with huma operations for all ten reminder and reminder-completion endpoints;
  depguard rules `reminders-*` (shown to fail); `routes.NewModules` extended. Old controller and
  tests removed. 14 module tests on real SQLite plus domain tests, using the fake clock for
  complete/upcoming. Frontend `Reminder`, `ReminderFormData`, `ReminderCompletion` from the
  generated types; shared `WithoutNull` moved to `src/types/utils.ts`; dead `*ReminderResponse`
  types removed. `make precommit` green.
- Step 13d: `backend/relationships` (domain `Relationship.Edit`, `BuildGraph` with the visibility
  rules; use cases; ports `Relationships`/`GraphSource`/`Store`; persistence/http adapters) with
  huma operations for the five relationship endpoints and `GET /graph`; depguard rules
  `relationships-*` (shown to fail); `routes.NewModules` extended. Old relationship and graph
  controllers, their tests and `models.RelationshipInput`/`GraphNode`/`GraphEdge`/`GraphResponse`
  removed; 10 module tests on real SQLite incl. isolation and the graph rules, plus domain tests.
  The contacts module's embedded relationship type is now `ContactRelationship` (schema name clash).
  Frontend `Relationship`, `IncomingRelationship`, form data, graph types derive from the generated
  types; dead hand-written relationship types removed. `make precommit` green.
- Step 13e: `backend/photos` (domain: safe-name rules and legacy-thumbnail decoding; use cases
  `OpenPhoto`, `OpenThumbnail`, `UploadPhoto`; ports `Contacts`/`Store`/`Files`; persistence, files
  and http adapters). The upload is a huma multipart operation; the two image downloads are plain
  Gin handlers inside the module's http adapter (they send bytes with ETag/Range support). Image
  processing moved to `adapters/files` with its tests (sizes, private modes, HEIC), module tests on
  real SQLite for scoping, traversal, legacy thumbnails, trashed contacts, replacing files and 401.
  The `thumbnailURL` copy in the contacts http adapter is gone: contacts persistence puts the URL
  into the domain contact (`models.ThumbnailURL` remains for trash and birthdays). depguard rules
  `photos-*` (shown to fail). There were no photo-specific arch exemptions to remove. `ProxyImage`
  moved to `controllers/image_proxy_controller.go` (not a photos concern, stays Gin).
  `make precommit` green.
- Step 13f: `backend/trash` (domain `Retention`/`PurgeAt`; use cases `ListTrash`, `RestoreContact`,
  `PurgeContact`, `EmptyTrash`; ports `Trash`/`Events`; persistence and webhooks adapters) with huma
  operations for the four trash endpoints; `contact.restored` webhook as before; depguard rules
  `trash-*` (shown to fail); `routes.NewModules` extended. Old trash controller, `models.Trashed*`
  types, `services.TrashPurgeAt`/`DefaultTrashRetentionDays` removed. 6 module tests on real SQLite
  (fake clock, isolation, files removed on purge, webhook payload, malformed ids, 401); the
  cross-module trash tests stay in `controllers`. Frontend `TrashedContact` and `TrashResponse`
  derive from the generated types. `make precommit` green.
- Step 13g1: `backend/users` (domain: supported-value rules; use cases; port `Preferences`;
  persistence and http adapters) with huma operations for `PATCH /users/language`,
  `PATCH /users/date-format`, `GET/PATCH /users/custom-fields` and
  `GET/PATCH /users/enabled-contact-fields`, all on the authenticated group; depguard rules
  `users-*` (shown to fail); `routes.NewModules` extended. Old handlers and
  `models.CustomFieldNamesInput`/ `EnabledContactFieldsInput` removed; 7 module tests on real SQLite
  (isolation between users, untouched password, null versus empty, wording of the unsupported-value
  errors, 401). Frontend `api/users.ts` types its requests and responses from the generated types.
  `make precommit` green.

## Decisions

- Step 12 is too large for one reviewable commit (10 endpoints, a 40-field entity,
  search/FTS/`fields` handling, and the whole frontend contact type). Split: **12a** (done) layout
  and lifecycle endpoints with the typed `ContactResponse`; **12b** `GET /contacts/{id}` (with
  `fields`), `POST /contacts`, `PUT /contacts/{id}`: domain rules for the denormalised primary
  email/phone/address, the `ContactInput` validation through `api.ValidateBody`, webhooks
  `contact.created/updated`; **12c** `GET /contacts` (search, FTS, circles, sort, paging, `fields`),
  `/contacts/random`, `/contacts/birthdays`, then delete the hand-written frontend `Contact` types
  and the remaining Gin handlers.
- 12b: `GET /contacts/{id}` without `fields` no longer embeds notes, activities and reminders (the
  typed `ContactResponse` has relationships only; the frontend always passes `fields` and those have
  own endpoints). The `photo_thumbnail` field name is no longer accepted by `fields` (it was never
  in the JSON, `json:"-"`). `models.Contact.BeforeSave` still syncs the primary values too, because
  CardDAV and imports save the model directly. The webhooks adapter reuses the HTTP adapter's
  `NewContactResponse` so payloads keep the old shape.
- 12b review: the full `GET /contacts/{id}` embeds notes, activities and reminders again (typed
  responses; the old zero-valued nested `contact`/`contacts` of those items is not reproduced) and
  answers `relationships: []` when there are none (`null` with `fields`). Validation error detail
  keys/messages now use JSON names (`emails[0].value`) instead of Go field names, as every huma
  operation does (ADR 0011). A failed read during update is worded "Failed to update contact".
- Step 13j/13k: remove the duplicate primary-value rule (`models.FormatAddress`,
  `Contact.BeforeSave`) once CardDAV and imports go through the contacts module.
- The webhooks adapter imports the HTTP adapter's `NewContactResponse` on purpose: one JSON shape
  for responses and payloads, and a shared presenter package would be a new layer for one function.
  Revisit when a second module publishes events.
- 12c: `models.ThumbnailURL` is duplicated in the HTTP adapter (`thumbnailURL`) until trash and
  birthdays stop using the model; remove then. The frontend `Contact` maps the API's `null` lists to
  absent (`WithoutNull`) as the old hand-written type did.
- 12c: the page size bounds (default 25, max 100) live in `app.DefaultLimit/MaxLimit` and are
  repeated in the `ListInput` docs of the HTTP adapter and in `controllers.GetPaginationParams`
  (still used by other lists); unify when the last list moves into a module.
- 13a: `PUT /notes/{id}` used GORM `Updates(struct)`, which skips a nil `contact_id`: a note cannot
  be made free-floating, but the old response claimed `contact_id: null`. Kept the first
  (`Note.Edit`, documented in the API reference); the response now shows what is stored.
  `GET /contacts/{id}/notes` answers `[]` (was possibly `null`) for no notes. A failed update now
  reports "Failed to update note" (the old code ignored the error). Page size bounds are repeated in
  the notes use case (see the 12c note).
- 13a review: the composition moved to `routes.NewModules(db, cfg)` (one place instead of five); ADR
  0012 and backend.md say so. The webhook payloads of `note.*` no longer carry the zero
  `"contact": {...}` object the GORM model serialised (nothing used it). `POST /contacts/{id}/notes`
  with an invalid body for an unknown contact now answers 400 (validation runs before the lookup),
  it used to answer 404. The notes webhooks adapter also imports the HTTP adapter's
  `NewNoteResponse`: with two modules doing it the pattern stays (one JSON shape for responses and
  payloads); extract a shared presenter only if a third kind of consumer appears.
- 13b: contacts inside an activity are now `{ID, firstname, lastname, circles}` only; before, the
  whole zero-valued contact object was serialised (all other fields empty). Same for the
  `activity.*` webhook payloads. The create and update responses and payloads now list the linked
  contacts in that form (create used to return the full contact rows).
  `GET /contacts/{id}/activities` lists by activity ID (it had no `ORDER BY`). Contacts are linked
  through the `activity_contacts` rows directly, not through GORM's association save. The update
  error says "Failed to save activity" as before. The frontend no longer reads `nickname` of an
  activity's contacts: the API never sent it, so the timeline line that showed it was dead.
- 13b review: a database error while listing a contact's activities says "Failed to retrieve
  activities for contact" for both the contact lookup and the list (the lookup used to say "Failed
  to retrieve contact"). Response types of the three modules are prefixed with the module's noun
  (`NoteSavedOutput`, ...) so the OpenAPI schema names are stable instead of `CreatedOutputBody1`.
- 13c, scheduler job: the daily e-mail job (`services.SendReminders`, lock, outbox, birthdays,
  templates) stays in `services`, deferred to 13i. It orchestrates users, birthdays and e-mail, none
  of which are modules yet, and touches reminders only through `models.Reminder`. It keeps the
  injected clock (ADR 0009) and its tests. Moving the "due by mail" query and `email_sent` marking
  behind a port is for 13i, when its other collaborators have ports. Review item.
- 13c, API: a reminder is returned without the zero-valued nested `contact` object; the body of
  create no longer accepts `completed`, `email_sent` or `last_sent` (they were taken as sent, now
  ignored); `by_mail` / `reoccur_from_completion` left out on update keep their value (as with GORM
  `Updates`), and on create fall back to the database default; update errors now surface ("Failed to
  update reminder"; the old code ignored them); marking a reminder complete and writing its timeline
  entry share one transaction (a failed entry is still only logged, now by the HTTP adapter with the
  request's IDs via `api.Logger`); an unknown recurrence is returned unchanged by `NextOccurrence`
  without the old warning log (domain has no logger); `GET /contacts/{id}/reminders` orders by ID,
  and answers `[]` for none. Deleting a completion distinguishes "retrieve" and "delete" failures as
  before. `services.addMonths/ addYears` stay for the Monica import, duplicated in the domain:
  remove with step 13j.
- 13d: `PUT`/`DELETE /contacts/{id}/relationships/{rid}` still look up only `rid` (the contact in
  the URL is not compared), as before: a relationship of the user can be changed through any contact
  URL. Left as is on purpose in a refactor; worth tightening later. Review item.
- 13d: `GET /graph` errors now use the AppError format
  (`Failed to fetch contacts/relationships/ activities` as `DATABASE_ERROR`); they were
  `{"error": "..."}` with status 500. Relationship lists are ordered by ID and answer `[]` for none;
  `related_contact`/`source_contact` carry only the fields that were selected before (the rest of
  the zero-valued contact object is gone); an update failure now surfaces ("Failed to update
  relationship"; `db.Save`'s error was ignored). JSON strings no longer escape `&` as `\u0026` (the
  huma encoder), so the thumbnail test compares `&`; the data is the same.
- 13e, API: the upload answers `{"message", "photo"}` instead of the whole contact (the frontend
  ignores the body; the contact is read from `GET /contacts/{id}`); an upload request without a
  `photo` file changes nothing (it used to re-save the contact, touching `updated_at`); an upload
  over the 10 MB body limit answers 400 "... request body too large" (the multipart form cannot be
  read); before, the failed read made the handler skip the file and answer 200 with nothing changed.
  The separate 10 MB file-size check was unreachable and is gone; `DEMO_MODE` is still read from the
  environment per request in the http adapter. Thumbnail and photo errors keep their wording per
  stage. Downloads use `http.ServeContent` (Range and Last-Modified for the full photo, as `c.File`
  did). Review item: whether the downloads should become huma stream responses once step 14 lists
  exceptions.
- 13e review: `UploadPhoto` processes the image outside the transaction (SQLite's write lock is not
  held while pictures are decoded and written) and removes the new files whenever the transaction
  fails.
- 13f, scope: the restore and purge logic (restore window for dependants, chunked purge, CardDAV
  tombstones, file deletes after each committed chunk, so slow I/O never holds the write lock) stays
  in `services.RestoreTrashedContact`/`PurgeTrashedContacts`/`ListTrashedContacts`. The contact sync
  (`restoreTrashedContactTx`, 13k) and the daily purge job (`PurgeExpiredTrash`, main with the
  injected clock) use them, and the module cannot be imported from `services` without a cycle. The
  persistence adapter calls them; the logic moves with 13k. The `contact.restored` payload is still
  the GORM contact (passed through the ports as an opaque event value) because the contacts module's
  response type is internal to it. `models.ThumbnailURL` stays: contacts, relationships and trash
  persistence and the birthday service still use it (it can go when the birthdays move). Behaviour
  kept: a non-numeric or zero `{id}` is a 404 "Contact" (tested), not a 400; the retention (and a
  missing photo directory falling back to `PROFILE_PHOTO_DIR`) is read once when the module is
  built, not per request. `trashPhotoDir` became `photoDir` in the backup controller, its only other
  user.
- Step 13g is split because it is large and security-critical: **13g1** user preferences (done),
  **13g2** sessions and change password, **13g3** TOTP, **13g4** register, login, logout, password
  reset, password strength and `GET /users/me` (public routes with the auth rate limit, cookies),
  **13g5** OIDC login and callback (redirects, stay Gin handlers inside the module, like the image
  downloads). The auth middleware, JWT and token-version logic stay in `middleware` and `services`
  until 13g4 shows what the module needs from them.
- 13g1: the preference writes now use the user ID from the authenticated context; the language and
  date-format handlers used to look the user up by `username`. A user that cannot be found answers
  401 for every preference (before: 401 for language and date format, 500 "query user" for the
  others); that cannot happen with a valid session. Writes update one column (`Select(...).Updates`)
  instead of `Save` of the whole user. The old language check never rejected anything:
  `i18n.IsValidLanguage` normalised every value to "en" (and was removed, no other caller), so "xx",
  "", "DE" and "de-DE" were stored as sent. Now only `en de it es fr` are accepted and anything else
  answers 400 "Unsupported language. Supported: en, de, it, es, fr" (the wording the old code meant
  to give; the date format message is unchanged). The settings page only offers the five codes, so
  no user sees a difference and there is no CHANGELOG line. A body that is not JSON, or empty, now
  gets the generic request error ("request body" / "validation failed" or "request body is
  required", 400) instead of "Invalid language value". Status codes verified by test: 200, 400, 401.
- 12a keeps behaviour: `GET /contacts/circles` still answers `null` when there are no circles;
  non-numeric `{id}` now answers `400` (a Gin string param used to reach the database); a missing
  user answers `401`.

- First converted endpoint is `GET /auth/oidc/config` (public, no database, used by the frontend),
  not `check-password-strength` (not used by the frontend). Error behaviour, per-route Gin
  middleware and the validators are covered by tests in `backend/api` instead.
- huma's `DefaultConfig` is not used: it adds `$schema` fields to responses and serves docs routes.

- Deviation in step 10: `backend/main.go` (the composition root) may import `controllers` in
  addition to `routes`; the depguard exemption names that exact path, not every `main.go`.

- Frontend baseline uses dependency-cruiser's known-violations file (exact from/to pairs) rather
  than `pathNot` globs, so an exemption names one import, not a whole file or folder.

- Clock injection: request-scoped services take a trailing `now time.Time`; long-running jobs take
  the `clock.Clock`; the two import session managers hold a `clock.Clock` (set once by
  `controllers.SetImportClock`) because their methods are many and long-lived. The gocron scheduler
  still uses the wall clock for when jobs start (documented in ADR 0009). The browser does not
  follow `FAKE_NOW`.

- Baseline via ESLint bulk suppressions, not per-file `off` entries: a file's existing violations
  are counted, so new ones in the same file fail. `eslint-suppressions.json` is in `.prettierignore`
  (the tool writes it).

- ESLint 9 vs `react-scripts` 5 (bundles ESLint 8): yarn nests ESLint 8 under react-scripts, but its
  `eslint-webpack-plugin` would pick the hoisted ESLint 9, so `start` and `build` set
  `DISABLE_ESLINT_PLUGIN=true` and the `eslintConfig` block in package.json is removed. Lint runs
  only through `make lint-frontend`. The build itself is unchanged.
- `react-hooks` v7 `recommended` adds React Compiler rules; only `rules-of-hooks` and
  `exhaustive-deps` (what `react-app` had) are enabled.
- `no-confusing-void-expression` auto-fix changed ~250 arrow handlers to block bodies (behaviour
  unchanged).

- CI sets `YARN: yarn` (runner yarn) because the Makefile default `corepack yarn` needs a
  `packageManager` field.
- golangci-lint job uses the action with `install-only: true`, then `make lint-go` (adds `go vet`);
  loses the action's lint cache.

- `test-coverage` is not in `precommit` yet (step 8 adds thresholds first).

## For review

- Step 7: `frontend/eslint-suppressions.json` hides 529 findings (mostly
  `no-confusing-void-expression` fixed; remaining: restrict-template-expressions 91,
  no-misused-promises 90, no-floating-promises 65, no-unsafe-* ~140, no-unnecessary-condition 61,
  no-deprecated 34). Later steps should shrink it. `no-autofocus` is exempted as a design choice to
  confirm.

- Step 4: CI cannot run locally. Check the first PR run: `install-only` of golangci-lint-action v8,
  `YARN: yarn`, `make fuzz` env vars.

## Questions

- `PUT`/`DELETE /contacts/{id}/relationships/{rid}` look up only `rid`; the contact in the URL is
  not compared with the relationship's contact (unchanged by step 13d). Tightening it would answer
  404 for a mismatch; decide whether clients rely on the loose behaviour.
- The frontend used to crash on an incoming relationship without `source_contact` (guarded in 13d by
  leaving such rows out; the API may omit it when the source contact is in the trash).

- Vitest timeouts (5000ms) recur under load when `make precommit` runs everything at once: step 13c
  first run, `ImportContactsDialog.test.tsx` ("a summary chip filters the rows...", "a filtered row
  stays visible..."), 6.1 s and 5.3 s; earlier in 13a two other files. A rerun passes. Step 13f: it
  failed three full runs in a row (about 5.2 s under load, 1 s alone), so
  `ImportContactsDialog.test.tsx` now has a 20 s timeout on its `describe` (MUI-heavy renders);
  other slow tests may need the same.

- A request body with `"date": "2026-03-01"` (no time) answers "date is required" for notes and
  activities, which is misleading (the value is there, it is not an RFC 3339 timestamp). Seen while
  testing step 13b; same cause in the old Gin handlers' `time.Time` binding is likely.

- E2E flake seen twice (step 8 and step 10, first run only, rerun green): `relationships.spec.ts`
  "Relationship ..." failed once in step 10. Cause not investigated; watch for it. Seen again in
  step 12a: `relationships.spec.ts` beforeEach got `failed to create test contact: 500` from
  `POST /contacts` (about one full precommit in three, reruns green). Likely SQLite contention right
  after the heavy checks; the 500 itself was not captured.

- One full `make precommit` run during step 8 had 2 E2E failures (52 passed); a rerun and a second
  full run passed 54/54. Names not captured. Possible flake; watch for it.

- `new Date().toISOString().slice(0, 10)` (and the older `.split('T')[0]`) gives the UTC date, so
  default dates in AddNote/AddActivity and the reminder `min` can show the wrong day near midnight.
  Pre-existing; not fixed.

- `frontend/package.json` has no `packageManager` field, so `corepack yarn` (Makefile default) needs
  CI to set `YARN=yarn`. Adding the field would remove that workaround; not done (out of scope).

- `apperrors.ErrNotFound(resource)` appends " not found", so
  `backend/services/import_session.go:96,100,107` and `monica_import_session.go:132,135,149` produce
  "Import session expired or not found not found". Not fixed (out of scope).

## Blocked
