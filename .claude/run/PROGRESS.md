# Progress

## Next step

Step 2: coverage audit (#107).

## Stack

<!-- step: branch, PR number (base) -->

Local run: no branches, no PRs.

## Log

- Steps 0 and 1: `make precommit` runs end to end in this environment, including `make test-e2e`
  (Playwright works without a workaround). `EditTimelineItemDialog` now searches contacts on the
  server (300 ms debounce, `limit: 100`, like `AddActivityDialog`); `allContacts` is gone from the
  dialog, `useTimelineEditing`, `ActivitiesPage`, `NetworkPage`, `ContactDetailPage`, `NotesPage`.
  Added Vitest test and an E2E test; the timeline edit button got an accessible name.

## Decisions

- Step 1: the E2E test cannot reproduce the 100-contact cap (few seeded contacts); the Vitest test
  covers the server search.

## For review

## Questions

- Contact search in the activity dialogs is duplicated in `AddActivityDialog` and
  `EditTimelineItemDialog`; a shared `useContactSearch` hook would remove it.
- The edit `IconButton` at `ActivitiesPage.tsx` (~line 284) has no accessible name.

## Blocked
