# Run plan

Decided with the user on 2026-10-19 from the open questions of the `coverage-and-search` run. Each
step: test first, own commit, `make precommit` green.

## Next steps

- [x] Copy the final https://github.com/omarkohl/meerkat-crm/issues/107 audit into the issue.
      Source: section "Final https://github.com/omarkohl/meerkat-crm/issues/107 audit" in
      `.claude/plans/coverage-and-search-summary.md`, in the parent of the commit that deletes it.
- [ ] https://github.com/omarkohl/meerkat-crm/issues/109 (merge contacts). Settle first: merge
      dialog and conflict UI, removal from synced CardDAV devices, undo or confirmation, API shape.
- [x] Birthday mail sent mark: store the last-sent date per user (migration), so a restart does not
      send it again.
- [ ] `contacts.add.requiredFields`: say only the first name is required (5 locales).
- [ ] `AddContactDialog`: a contact added on its birthday gets its first reminder this year.
- [ ] `AddContactDialog`: trim the first name before sending.
- [ ] `ActivitiesPage` edit `IconButton`: add an accessible name (5 locales).
- [ ] `ImportContactsDialog` drop zone: keyboard access (role, `tabIndex`, Enter and Space).
- [ ] Search fallback (LIKE): fold case for non-ASCII text (for example `Ü`).
- [ ] Add a per-route isolation map for ID-taking GET routes in `routes/invariants_test.go`.
- [ ] One `registerCardDAVRoutes` function for the server and the test router (with
      `CardDAVRateLimitMiddleware`).
- [ ] One `useContactSearch` hook for `AddActivityDialog` and `EditTimelineItemDialog`.
- [ ] Test `fetchNeededCards` in the default suite with a fake CardDAV server.
- [ ] Test the `oidcLogin` 500 branches with a fake `Identity` (app-level test).
- [ ] E2E: open a note without a contact from the search results page; check the filtered Notes
      list.
- [ ] Research: coverage for E2E tests (Go `-cover` with `GOCOVERDIR`, frontend V8 or Istanbul
      through Playwright) and a sensible integration-test layer. Write findings, then decide.
- [ ] Frontend unit tests for the untested logic (`useReminderManagement`, `ContactDetailPage`,
      others as the research shows). Test user-visible behaviour only; no coupling to internals. Do
      this after the coverage research.
- [ ] Bug report for `emersion/go-webdav`. Verified in v0.7.0: `carddav.Backend.DeleteAddressObject`
      takes no `If-Match`, while the generic `webdav` server passes `IfMatch` to `RemoveAll`. A
      stale conditional DELETE succeeds, but RFC 7232 asks for 412. Check the newest version and the
      open issues first. Show the draft to the user; do not file it.
- [ ] Search follow-ups: a keyboard shortcut to focus the header search (`/` or `Ctrl+K`, see
      IMPROVEMENTS "Quick-capture command palette"), reminders in the search.
- [ ] Pick further work from `IMPROVEMENTS.md` (open items only).

## Settled, no step

- CardDAV login with an API token as password: part of
  https://github.com/omarkohl/meerkat-crm/issues/130 (app passwords), not a separate step.
- `AddNoteDialog`, `AddRelationshipDialog`, `ProfilePictureUploadDialog`, `ContactHeader`,
  `ContactTimeline` and the settings components: no unit tests (thin; E2E covers them).

## Open Questions

- https://github.com/omarkohl/meerkat-crm/issues/130: should an API token with a CardDAV scope be
  the app-password model, or a separate model?
