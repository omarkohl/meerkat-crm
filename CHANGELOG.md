# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Versions `X.Y.Z` are upstream releases of
[fbuchner/meerkat-crm](https://github.com/fbuchner/meerkat-crm). Versions `X.Y.Z-omk.N` are releases
of this fork (see [FORK.md](FORK.md)).

## [Unreleased]

### Added

- **Merge duplicate contacts** (#109): select contacts in the list or use "Merge with…" on a
  contact, resolve differing values and combine their data. The others go to the trash.
- **Search contacts, notes and activities from the header** (#118): grouped results with the match
  marked and named, over every contact field, and a results page with a tab per group.
- **Search box on the contact list**: finds contacts by any field. Contacts you selected for a merge
  stay selected while you search, and one button clears the selection.

### Fixed

- **Birthday e-mail is sent once a day**: restarting the server on a birthday sent it again.
- **Imports skip invalid rows and survive broken vCards** (#107): rows without a first name were
  still added, and one unreadable vCard in a file shifted every later card or crashed the import.
- **CardDAV clients no longer overwrite newer changes** (#107): a quick second save could replace
  newer changes, "create only" saves replaced existing contacts, and missing contacts gave an error.
- **Asking CardDAV for another user's address book answers "not found"**: it answered with a server
  error.
- **Editing an activity finds every contact** (#140): the contact field only offered some contacts
  when you had many. It now searches all of them as you type.

### Security

- **Docker images include current Alpine fixes**: published images now contain package security
  fixes released after the base image.
- **Safer loading of pictures from web addresses**: only the standard web ports are allowed, and the
  profile picture dialog loads at most 30 pictures per minute.

### Internal

- **API description**: `docs/openapi.json` describes the JSON API and is generated from the code.

## [1.7.0-omk.2] - 2026-10-17

### Fixed

- **Saving no longer fails while other changes are saved**: concurrent saves sometimes failed with a
  "database is locked" error.

### Changed

- **Frontend build variable renamed**: images built from source take `VITE_API_URL` instead of
  `REACT_APP_API_URL`.

### Internal

- **`FAKE_NOW`** sets the app's clock, to test and preview time-dependent behaviour.
- **Vite** replaces Create React App to build and serve the frontend.
- **npm replaces Yarn**: development and builds need only Node.js.
- **Restructuring**: extensive internal restructuring (hexagonal architecture)

## [1.7.0-omk.1] - 2026-10-16

First fork release. Based on upstream 1.7.0 plus later upstream changes.

### Added

- **Two-factor login**: sign in with a TOTP authenticator app.
- **Login sessions and audit log**: see and end your sessions, and review account activity.
- **Trash**: restore or permanently delete removed contacts.
- **Calendar feed**: subscribe to birthdays and reminders, and see when the feed was last fetched.
- **Scheduled backups**: built-in backups that an admin can download.
- **Faster search**: full-text search.
- **Import preview**: filter, sort and set values for many rows at once, and see which rows were
  skipped and why.
- **vCard notes**: the vCard NOTE field is imported and synced.
- **Collapsible sidebar**: collapse the sidebar to icons.
- **Restore prompt on import**: re-importing a deleted contact asks whether to restore it.
- **Metrics and retries**: optional Prometheus metrics; webhooks and emails are retried on failure.

### Changed

- **Dates follow the configured format**: all date inputs and filters now use it, not the browser
  locale.
- **One placeholder for empty values**: contact fields, lists and the audit log all show the same
  mark.
- **Contact photos** are stored as files, not in the database.

### Fixed

- **Import of incomplete vCards**: nameless or partly invalid cards are imported, not rejected.
- **vCard 2.1 and Apple files** import phones, emails and labels correctly.
- **Duplicate phones, emails, URLs and IMs** are dropped on import.
- **Add activity dialog** preselects the contact.
- **Header search** clears on navigation and Escape.
- **Deleted users** lose access at once.
- **Private contact photos** keep correct file permissions.
- **Monica import timeout** is longer, and a private Monica URL can be allowed.
- **Server port setting** uses the documented name.

### Security

- **Private-address blocking** is one setting, `BLOCK_PRIVATE_URLS`.
- **Updated dependencies and images** fix known vulnerabilities.

### Internal

- **Fork images** are published to `ghcr.io/omarkohl` and need an explicit `IMAGE_TAG`.
- **`make dev`** starts backend and frontend with no setup.
- **CI** runs the race detector, linters, vulnerability scans and image attestation.

## [1.7.0] - 2026-08-12

### Added

- **Monica import**: import assistant for Monica data.
- **CardDAV client**: Meerkat can connect to your own CardDAV server.
- **Network graph** remembers your filter choices.
- **New contact from relationship dialog**: create a contact while adding a relationship.

### Changed

- **Improved SSO login.**
- **Improved Docker builds** and guidance on auth cookies.

### Fixed

- **Batch import of vCard files** failed in some cases.

## [1.6.2] - 2026-07-25

### Fixed

- **All-in-one image** did not work on ARM systems.

## [1.6.1] - 2026-07-14

### Fixed

- **Docker health check** failed on some systems.

## [1.6.0] - 2026-07-12

### Added

- **French** language.
- **All-in-one Docker image** with frontend and backend in one container.
- **Calendar sync.**

### Changed

- **ISO date format** is available.
- **Line breaks** show in text fields.

## [1.5.0] - 2026-06-23

### Added

- **Spanish** language.
- **More contact data**: extended fields compatible with CardDAV and vCard, with multi-value import.
- **Email via SMTP.**
- **Today's birthdays** are highlighted on the dashboard.
- **Custom address types.**

### Changed

- **Integrations page** layout matches the rest of the UI.

## [1.4.1] - 2026-05-23

### Fixed

- **Reminder times**: users can set their timezone.

## [1.4.0] - 2026-05-19

### Added

- **Webhooks** for actions in Meerkat.

### Fixed

- **Demo password** in the live demo link.

## [1.3.6] - 2026-05-04

### Fixed

- **CardDAV** has its own rate limit.

## [1.3.5] - 2026-04-27

### Added

- **Network chart**: center on a contact and filter by circle.

### Changed

- **Italian translation** updated.

## [1.3.4] - 2026-04-17

### Added

- **Circle dropdown** filters as you type.

### Changed

- **API tokens** are available to all users, not only admins.

### Fixed

- **Contact list** links open in a new tab with Cmd/Ctrl-click.

## [1.3.3] - 2026-04-13

### Added

- **Disable registration**: stop new users from signing up.

### Fixed

- **Wrong date** in timezones behind UTC.

## [1.3.2] - 2026-04-11

### Changed

- **OIDC email setting** documentation updated.

## [1.3.1] - 2026-04-10

### Added

- **OIDC**: option to ignore email verification.

## [1.3.0] - 2026-04-10

### Added

- **OIDC** single sign-on.
- **Pets** as a relationship type.

## [1.2.3] - 2026-04-06

### Fixed

- **ARM64 images** did not work.

## [1.2.2] - 2026-04-06

### Added

- **ARM64** Docker images.

## [1.2.1] - 2026-04-05

### Added

- **Italian** language.

### Fixed

- **Dialogs** did not close correctly.

## [1.2.0] - 2026-04-03

### Added

- **API tokens.**
- **Custom user and group IDs** in the container.

### Changed

- **Usernames** can be one character long.

## [1.1.0] - 2026-03-31

### Added

- **Birth date input** inserts separators automatically.

### Changed

- **Passwords** are limited to 72 characters.

### Fixed

- **`/health` endpoint** is reachable through the frontend.

[Unreleased]: https://github.com/omarkohl/meerkat-crm/compare/v1.7.0-omk.1...HEAD
[1.7.0-omk.2]: https://github.com/omarkohl/meerkat-crm/compare/v1.7.0-omk.1...v1.7.0-omk.2
[1.7.0-omk.1]: https://github.com/omarkohl/meerkat-crm/compare/v1.7.0...v1.7.0-omk.1
[1.7.0]: https://github.com/fbuchner/meerkat-crm/compare/v1.6.2...v1.7.0
[1.6.2]: https://github.com/fbuchner/meerkat-crm/compare/v1.6.1...v1.6.2
[1.6.1]: https://github.com/fbuchner/meerkat-crm/compare/v1.6.0...v1.6.1
[1.6.0]: https://github.com/fbuchner/meerkat-crm/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/fbuchner/meerkat-crm/compare/v1.4.1...v1.5.0
[1.4.1]: https://github.com/fbuchner/meerkat-crm/compare/v1.4.0...v1.4.1
[1.4.0]: https://github.com/fbuchner/meerkat-crm/compare/v1.3.6...v1.4.0
[1.3.6]: https://github.com/fbuchner/meerkat-crm/compare/v1.3.5...v1.3.6
[1.3.5]: https://github.com/fbuchner/meerkat-crm/compare/v1.3.4...v1.3.5
[1.3.4]: https://github.com/fbuchner/meerkat-crm/compare/v1.3.3...v1.3.4
[1.3.3]: https://github.com/fbuchner/meerkat-crm/compare/v1.3.2...v1.3.3
[1.3.2]: https://github.com/fbuchner/meerkat-crm/compare/v1.3.1...v1.3.2
[1.3.1]: https://github.com/fbuchner/meerkat-crm/compare/v1.3.0...v1.3.1
[1.3.0]: https://github.com/fbuchner/meerkat-crm/compare/v1.2.3...v1.3.0
[1.2.3]: https://github.com/fbuchner/meerkat-crm/compare/v1.2.2...v1.2.3
[1.2.2]: https://github.com/fbuchner/meerkat-crm/compare/v1.2.1...v1.2.2
[1.2.1]: https://github.com/fbuchner/meerkat-crm/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/fbuchner/meerkat-crm/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/fbuchner/meerkat-crm/compare/v1.0.0...v1.1.0
