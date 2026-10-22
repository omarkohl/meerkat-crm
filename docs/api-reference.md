---
title: API Reference
nav_order: 8
has_children: false
---

# API Reference

The machine-readable contract of the JSON API is [openapi.json](openapi.json) (OpenAPI 3.1,
generated from the code by `make openapi`). It is the source of truth for request and response
shapes; this page adds the behaviour around them. Routes that send files, images or redirects are
not in the spec.

## Base URL

```
/api/v1
```

## Authentication

Auth endpoints (`/login`, `/register`, `/logout`, `/password-reset/*`, `/auth/oidc/*`) and the
calendar feed (`/ics/<token>.ics`, authenticated by the token in its URL) are public. All other
endpoints require authentication.

Admin endpoints (`/admin/*`) additionally require the user to have the admin flag set. API tokens
are refused there, even for admins.

Login sets an httpOnly JWT cookie. Subsequent requests must include it (i.e. send with
`credentials: 'include'`). The cookie name and domain are configured via `COOKIE_DOMAIN` and
`COOKIE_SECURE`.

Alternatively, API tokens can be used in the `Authorization` header:

```
Authorization: Bearer meerkat_<token>
```

API tokens are created and managed via the `/api-tokens` endpoints (see [API Tokens](#api-tokens)).
Tokens always have normal user rights: they are refused on `/admin/*` endpoints. The plaintext token
is only returned once at creation time.

## Error Responses

All errors follow the same structure:

```json
{
  "code": "NOT_FOUND",
  "message": "Contact not found",
  "details": {}
}
```

Common error codes:

| Code                  | HTTP status |
| --------------------- | ----------- |
| `UNAUTHORIZED`        | 401         |
| `INVALID_CREDENTIALS` | 401         |
| `TOKEN_EXPIRED`       | 401         |
| `FORBIDDEN`           | 403         |
| `NOT_FOUND`           | 404         |
| `VALIDATION_ERROR`    | 400         |
| `INVALID_INPUT`       | 400         |
| `ALREADY_EXISTS`      | 409         |
| `RATE_LIMIT_EXCEEDED` | 429         |
| `INTERNAL_ERROR`      | 500         |

## Request IDs

Every request and response carries an `X-Request-ID` header created by the middleware.

---

## Endpoints

### Auth

| Method | Path                       | Description                                                                                                                                                        |
| ------ | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `POST` | `/register`                | Create a new user account                                                                                                                                          |
| `POST` | `/login`                   | Authenticate and set session cookie                                                                                                                                |
| `POST` | `/logout`                  | Clear session cookie                                                                                                                                               |
| `POST` | `/check-password-strength` | Validate a password without registering                                                                                                                            |
| `POST` | `/password-reset/request`  | Send a password reset email                                                                                                                                        |
| `POST` | `/password-reset/confirm`  | Apply a password reset token                                                                                                                                       |
| `GET`  | `/auth/oidc/config`        | Public. `{"enabled": bool, "provider_name": ...}` (`provider_name` only when SSO is enabled)                                                                       |
| `GET`  | `/auth/oidc/login`         | Start the SSO login: redirects the browser to the identity provider. Only registered when OIDC is enabled, see [Single Sign-On](deployment.md#single-sign-on-oidc) |
| `GET`  | `/auth/oidc/callback`      | SSO redirect target: completes the login, sets the session cookie and redirects to the app. Only registered when OIDC is enabled                                   |

### Users

| Method  | Path                            | Description                                                                  |
| ------- | ------------------------------- | ---------------------------------------------------------------------------- |
| `GET`   | `/users/me`                     | Get the current user                                                         |
| `POST`  | `/users/change-password`        | Change password                                                              |
| `PATCH` | `/users/language`               | Update UI language preference                                                |
| `PATCH` | `/users/date-format`            | Update date format preference                                                |
| `GET`   | `/users/custom-fields`          | Get custom field names                                                       |
| `PATCH` | `/users/custom-fields`          | Update custom field names                                                    |
| `GET`   | `/users/enabled-contact-fields` | Optional contact fields shown in the UI: `{"enabled_contact_fields": [...]}` |
| `PATCH` | `/users/enabled-contact-fields` | Replace that list. Body: `{"fields": ["..."]}` (each at most 50 characters)  |

### Contacts

| Method   | Path                            | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| -------- | ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`    | `/contacts`                     | List contacts (supports search and circle filter). `search` is a full-text search over name, nickname, e-mail addresses and phone numbers: case and accents are ignored (`zoe` finds `Zoë`), every word must match the start of a word, in any order (`mül zo`). Without `sort` the best match comes first; with `sort` that order wins. Text with no letters or digits (`+`), only digits and phone punctuation (`795550`) and Chinese, Japanese or Korean text are plain substring searches. Terms are cut at 200 characters and 32 words. `search_all=true` searches every text field of a contact, like `/search`, and ranks name matches first. Notes and activities (`search` on `/notes` and `/activities`) use the same matching but stay in date order, only contacts are ranked |
| `POST`   | `/contacts`                     | Create a contact                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `GET`    | `/contacts/:id`                 | Get a contact with its relationships (`fields=a,b` returns only those fields, unknown names are ignored; notes, activities and reminders have their own endpoints)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `PUT`    | `/contacts/:id`                 | Update a contact                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `DELETE` | `/contacts/:id`                 | Delete a contact (moves it to the trash, see [Trash](#trash))                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `POST`   | `/contacts/:id/archive`         | Archive a contact                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `POST`   | `/contacts/:id/unarchive`       | Unarchive a contact                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `GET`    | `/contacts/circles`             | List all circles in use                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `GET`    | `/contacts/random`              | Get five random contacts                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `GET`    | `/contacts/birthdays`           | Get upcoming birthdays                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `POST`   | `/contacts/:id/profile_picture` | Upload a profile picture (multipart, field `photo`; JPEG, PNG or HEIC). Answers `{"message", "photo"}`; a request without a file changes nothing; an upload over 10 MB answers `400`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `GET`    | `/contacts/:id/profile_picture` | Get a contact's profile picture. With `?thumbnail=true`: the 48px thumbnail (JPEG) with an `ETag`, `Cache-Control: private, max-age=31536000, immutable` and `304` support                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `GET`    | `/proxy/image`                  | Proxy an external image URL for upload preview                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |

### Images

| Method | Path                   | Description                                                                                                                                                                                                                                                                       |
| ------ | ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `POST` | `/images`              | Upload an image (multipart, field `file`; JPEG, PNG, HEIC, WebP or GIF, at most 25 MB). Answers `201` with the image (`id`, `width`, `height`, `mime`, `bytes`, `taken_at`, `crop`, `url`, `thumb_url`). The image is pending until it is saved with a note, activity or contact. |
| `GET`  | `/images/:id/:variant` | The bytes of an image of the user. `variant` is `image` or `thumb`. With `?download=1` the response is an attachment. It has an `ETag`, `Cache-Control: private, max-age=31536000, immutable` and `304` support                                                                   |

The `photo_thumbnail` field of contact lists, the trash, birthdays and the graph is the URL of that
thumbnail (`/api/v1/contacts/:id/profile_picture?thumbnail=true&v=<name>`, empty when the contact
has none), not the image itself; `v` changes whenever the picture does, so browsers can cache it for
good. It needs the same login as the API.

### Search

| Method | Path      | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ------ | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `GET`  | `/search` | Search contacts, notes and activities at once. Query: `q` (required, at most 200 characters), `type` (`contacts`, `notes` or `activities`), `limit` (1 to 50, default 5), `offset` (default 0, needs `type`). Without `type` the answer has `contacts`, `notes` and `activities`, each `{"total", "items"}` with the first `limit` results; with `type` only that group is answered and `offset` pages through it. Matching is the one of the contact search (case and accents ignored, start of every word, digits and CJK text as substrings). Contacts rank a name match above an e-mail or phone match, notes and activities by relevance. The matched text comes as `[{"text", "match"}]` segments (`name`, `snippet`, `title`); an activity lists the names of its contacts in `contacts`, never as HTML; a contact says where it matched in `matched_field` (`name`, `email`, `phone`, `organization`, `job_title`, `department`, `role`, `middle_name`, `address`, `circles`, `how_we_met`, `food_preference`, `work_information`, `contact_information`, `note` (the CardDAV note) or `custom_field`, whose name is in `matched_field_name`). Names, e-mail addresses, phone numbers and all those fields are searched; custom field names are not, only their values. The contact list's `search` covers names, e-mail addresses and phone numbers only, unless it is sent with `search_all=true`. Trashed contacts and their notes are not found. |

### Trash

Deleting a contact moves it to the trash: the contact and its notes, reminders and relationships are
soft-deleted, while its activity links and photo files are kept. The trash is private to each user.
Contacts that have been in the trash longer than `TRASH_RETENTION_DAYS` (default 30) are purged
automatically by a daily job (03:30 in `REMINDER_TIMEZONE`). Contacts in the trash are hidden
everywhere else (lists, search, circles, birthdays, graph, ICS feed, reminders, CardDAV).

| Method   | Path                          | Description                                                                                                                                                                                                                                                                                                                                                                                                           |
| -------- | ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`    | `/trash/contacts`             | List the trash, newest deletion first: `{"contacts": [{"id", "firstname", "lastname", "nickname", "email", "photo_thumbnail", "deleted_at", "purge_at"}], "retention_days": 30}`                                                                                                                                                                                                                                      |
| `POST`   | `/trash/contacts/:id/restore` | Restore a contact with the notes, reminders and relationships deleted with it, and fire the `contact.restored` webhook event (payload: the contact, like `contact.created`). `404` if it is not in the user's trash                                                                                                                                                                                                   |
| `DELETE` | `/trash/contacts/:id`         | Delete a trashed contact permanently, with its dependants and photo files. `404` if it is not in the user's trash (live contacts cannot be purged here). No webhook event: `contact.deleted` fired when the contact went to the trash. A CardDAV link of the contact is kept as a tombstone: pull-only connections never import the remote card again, connections that push delete it on the server at the next sync |
| `DELETE` | `/trash/contacts`             | Empty the user's trash: `{"message": ..., "purged": <count>}`                                                                                                                                                                                                                                                                                                                                                         |

### Relationships

| Method   | Path                                   | Description                                                                                  |
| -------- | -------------------------------------- | -------------------------------------------------------------------------------------------- |
| `GET`    | `/contacts/:id/relationships`          | List outgoing relationships                                                                  |
| `GET`    | `/contacts/:id/incoming-relationships` | List incoming relationships                                                                  |
| `POST`   | `/contacts/:id/relationships`          | Create a relationship                                                                        |
| `PUT`    | `/contacts/:id/relationships/:rid`     | Update a relationship. Only `:rid` is looked up; leaving out `related_contact_id` unlinks it |
| `DELETE` | `/contacts/:id/relationships/:rid`     | Delete a relationship                                                                        |

### Notes

| Method   | Path                  | Description                                                      |
| -------- | --------------------- | ---------------------------------------------------------------- |
| `GET`    | `/contacts/:id/notes` | List notes for a contact                                         |
| `POST`   | `/contacts/:id/notes` | Create a note for a contact                                      |
| `GET`    | `/notes`              | List unassigned notes                                            |
| `POST`   | `/notes`              | Create an unassigned note                                        |
| `GET`    | `/notes/:id`          | Get a note                                                       |
| `PUT`    | `/notes/:id`          | Update a note. Leaving out `contact_id` keeps the note's contact |
| `DELETE` | `/notes/:id`          | Delete a note                                                    |

A note has `images` (the image objects of [Images](#images), in order). On create and update the
body can carry `image_ids`: IDs of uploaded images, at most 20. On update, leaving it out keeps the
images and an empty list removes them. `content` may be empty when the note has an image; a note
needs text or an image. Deleting a note deletes its images.

### Activities

The optional `end_date` is the last day of an activity that spans several days. It may not be before
`date`.

| Method   | Path                       | Description                                                                                                                                      |
| -------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `GET`    | `/activities`              | List all activities. `fromDate` and `toDate` match activities with any day in the range                                                          |
| `POST`   | `/activities`              | Create an activity                                                                                                                               |
| `GET`    | `/activities/:id`          | Get an activity                                                                                                                                  |
| `PUT`    | `/activities/:id`          | Update an activity. Leaving out `contact_ids` keeps its contacts, an empty list removes them. Leaving out `end_date` keeps it, `null` removes it |
| `DELETE` | `/activities/:id`          | Delete an activity                                                                                                                               |
| `GET`    | `/contacts/:id/activities` | List activities for a contact                                                                                                                    |

An activity has `images`, and its body takes `image_ids`, in the same way as a note: at most 20,
leaving them out on update keeps them, an empty list removes them. Deleting an activity deletes its
images.

### Reminders

| Method   | Path                                 | Description                                                                                                                                              |
| -------- | ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`    | `/reminders`                         | List all reminders                                                                                                                                       |
| `GET`    | `/reminders/upcoming`                | List upcoming reminders (used by dashboard)                                                                                                              |
| `GET`    | `/reminders/:id`                     | Get a reminder                                                                                                                                           |
| `PUT`    | `/reminders/:id`                     | Update a reminder. Leaving out `by_mail` or `reoccur_from_completion` keeps the current value                                                            |
| `DELETE` | `/reminders/:id`                     | Delete a reminder                                                                                                                                        |
| `POST`   | `/reminders/:id/complete`            | Mark a reminder complete (creates timeline entry); `?skip=true` skips without one. A recurring reminder is scheduled again, a `once` reminder is deleted |
| `GET`    | `/contacts/:id/reminders`            | List reminders for a contact                                                                                                                             |
| `POST`   | `/contacts/:id/reminders`            | Create a reminder for a contact                                                                                                                          |
| `GET`    | `/contacts/:id/reminder-completions` | List completion history for a contact (timeline entries)                                                                                                 |
| `DELETE` | `/reminder-completions/:id`          | Delete a completion entry                                                                                                                                |

### Import

| Method | Path                           | Description                                                                                                                                                              |
| ------ | ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `POST` | `/contacts/import/upload`      | Upload a CSV file, returns parsed preview data                                                                                                                           |
| `POST` | `/contacts/import/preview`     | Apply column mapping, returns contacts with duplicate detection                                                                                                          |
| `POST` | `/contacts/import/confirm`     | Execute the import with per-row decisions                                                                                                                                |
| `POST` | `/contacts/import/vcf/upload`  | Upload a VCF file, returns contacts with duplicate detection. Invalid emails, phones and dates are dropped and listed in each row's `warnings`                           |
| `POST` | `/contacts/import/vcf/confirm` | Execute the VCF import. Like `/contacts/import/confirm`, the result lists each skipped row with its reason in `skipped_rows`. The `photos_queued` photos load afterwards |
| `GET`  | `/contacts/import/vcf/status`  | Progress of the photos of a confirmed import. Query parameter `session_id`; `404` when the import has no photos or has expired                                           |

#### Monica CRM import

Imports contacts from a [Monica](https://www.monicahq.com/) instance through its REST API. The steps
share a server-side session (`session_id`) that belongs to one user and expires after a while.

| Method   | Path                              | Description                                                                                                                                                                         |
| -------- | --------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `POST`   | `/contacts/import/monica/connect` | Body `{"base_url", "api_token"}`. Checks the connection and returns a `session_id`. Private addresses are refused when `BLOCK_PRIVATE_URLS` (or `MONICA_BLOCK_PRIVATE_URLS`) is set |
| `POST`   | `/contacts/import/monica/fetch`   | Body `{"session_id", "include_relationships", "include_extras", "collapse_reciprocal_relationships"}`. Starts fetching in the background (`202`)                                    |
| `GET`    | `/contacts/import/monica/status`  | Progress of the fetch. Query parameter `session_id` is required (`400` without)                                                                                                     |
| `GET`    | `/contacts/import/monica/preview` | The fetched contacts with duplicate detection. Query parameter `session_id` is required (`400` without)                                                                             |
| `POST`   | `/contacts/import/monica/confirm` | Body `{"session_id", "actions": [...]}` with a decision per row. Executes the import                                                                                                |
| `DELETE` | `/contacts/import/monica/session` | Cancel and discard the session. Query parameter `session_id` is required (`400` without)                                                                                            |

### Export

| Method | Path          | Description                                    |
| ------ | ------------- | ---------------------------------------------- |
| `GET`  | `/export`     | Download all data as CSV                       |
| `GET`  | `/export/vcf` | Download all contacts as VCF (includes photos) |

### Network

| Method | Path     | Description                    |
| ------ | -------- | ------------------------------ |
| `GET`  | `/graph` | Get contact network graph data |

### Webhooks

Webhooks send a signed HTTP `POST` to your URL when something happens. The `secret` used to sign
deliveries is generated by the server and returned only when the webhook is created.

| Method   | Path                       | Description                                                                                                                                                                                                                                                                                                                      |
| -------- | -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`    | `/webhooks`                | List the user's webhooks: `{"webhooks": [...]}`                                                                                                                                                                                                                                                                                  |
| `POST`   | `/webhooks`                | Create a webhook. Body: `{"name", "url", "events": [...], "is_active"}`. Returns `201` with the `secret` once                                                                                                                                                                                                                    |
| `GET`    | `/webhooks/:id`            | Get a webhook                                                                                                                                                                                                                                                                                                                    |
| `PUT`    | `/webhooks/:id`            | Update name, URL, events and active flag                                                                                                                                                                                                                                                                                         |
| `DELETE` | `/webhooks/:id`            | Delete a webhook                                                                                                                                                                                                                                                                                                                 |
| `POST`   | `/webhooks/:id/test`       | Send a test event now: `{"delivery": {...}}`                                                                                                                                                                                                                                                                                     |
| `GET`    | `/webhooks/:id/deliveries` | Last 50 deliveries, newest first: `{"deliveries": [{"id", "webhook_id", "event_type", "status_code", "error", "attempts", "status", "next_retry_at", "created_at"}]}`. `status` is `pending` (waiting for a try or retry, `next_retry_at` says when), `delivered` or `dead` (gave up). One entry per event, updated on every try |

Deliveries are queued in the database first and retried with backoff (see
[Delivery queue](deployment.md#delivery-queue-webhooks-and-email)). A user can have at most 20
webhooks (`409` beyond that). URLs pointing at private addresses are refused when
`BLOCK_PRIVATE_URLS` (or `WEBHOOK_BLOCK_PRIVATE_URLS`) is set.

Events: `contact.created`, `contact.updated`, `contact.deleted`, `contact.restored`, `note.created`,
`note.updated`, `note.deleted`, `activity.created`, `activity.updated`, `activity.deleted`,
`reminder.triggered`, `birthday.occurred`.

### Calendar Subscriptions

Import events of a CalDAV or iCalendar URL as activities. An event that spans several days gets an
`end_date`. Stored passwords are encrypted and never returned.

| Method   | Path                  | Description                                                                                                                                                                                                                                                            |
| -------- | --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`    | `/calendars`          | List the user's subscriptions: `{"calendars": [...]}`                                                                                                                                                                                                                  |
| `POST`   | `/calendars`          | Create a subscription. Body: `{"name", "url", "username", "password", "sync_enabled", "past_days", "future_days"}` (`201`)                                                                                                                                             |
| `PUT`    | `/calendars/:id`      | Update a subscription. `clear_password: true` removes the stored password. If the URL moves to another server (other host, port or scheme) while a password is stored, the request needs a new `password` (or `clear_password`): otherwise `400` with field `password` |
| `DELETE` | `/calendars/:id`      | Delete a subscription                                                                                                                                                                                                                                                  |
| `POST`   | `/calendars/:id/sync` | Sync now: `{"message", "created", "updated", "skipped"}`                                                                                                                                                                                                               |

A user can have at most 10 subscriptions (`409` beyond that). `PUT` replaces the subscription, so
the body needs at least `name` and `url`. URLs pointing at private addresses are refused when
`BLOCK_PRIVATE_URLS` (or `CALDAV_BLOCK_PRIVATE_URLS`) is set.

### CardDAV Connection

Syncs contacts with an external CardDAV server (Nextcloud, Radicale, ...). Each user has at most one
connection. This is the client side; Meerkat's own CardDAV server is described in
[CardDAV](carddav.md). Stored passwords are encrypted and never returned. Private addresses are
refused when `BLOCK_PRIVATE_URLS` (or `CARDDAV_BLOCK_PRIVATE_URLS`, default:
`CALDAV_BLOCK_PRIVATE_URLS`) is set.

| Method   | Path                       | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| -------- | -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`    | `/carddav/connection`      | Get the user's connection (`404` if none)                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `PUT`    | `/carddav/connection`      | Create (`201`) or update (`200`) the connection. Body: `{"base_url", "address_book_path", "address_book_name", "username", "password", "clear_password", "direction", "sync_enabled"}` with `direction` one of `two_way`, `pull`, `push`. On update, an empty `password` keeps the stored one only while the server stays the same (host, port and scheme); a change of server without a new `password` (or `clear_password`) answers `400` with field `password` |
| `DELETE` | `/carddav/connection`      | Remove the connection and its links                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `POST`   | `/carddav/discover`        | Body `{"base_url", "username", "password"}`. Lists the address books on the server: `{"address_books": [...]}`. An empty `password` reuses the stored one, but only when `base_url` has the same host, port and scheme as the saved connection                                                                                                                                                                                                                    |
| `POST`   | `/carddav/connection/sync` | Start a sync in the background (`202`)                                                                                                                                                                                                                                                                                                                                                                                                                            |

### Sessions

Every browser login is a session; a revoked or expired session is refused even though its token has
not expired yet. These endpoints are refused for API tokens (`403`).

| Method   | Path                   | Description                                                                                                                                                                                  |
| -------- | ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`    | `/sessions`            | The user's live sessions, newest first: `{"sessions": [{"id", "user_agent", "ip", "created_at", "last_seen_at", "expires_at", "current"}]}`. `last_seen_at` is updated at most once a minute |
| `DELETE` | `/sessions/:id`        | Sign one session out (`404` for an unknown, foreign or ended id). Revoking the current session is a logout                                                                                   |
| `POST`   | `/sessions/revoke-all` | Sign out everywhere: revokes all sessions, this one included, and invalidates older tokens (token version). API tokens and the calendar feed stay valid                                      |

Changing the password through `/users/change-password` signs out all other browsers (the current one
gets a new session); resetting it by e-mail or an admin setting a new password signs out everywhere.

### Two-factor authentication (TOTP)

Optional per user. Enrolment is two steps so a broken app cannot lock you out: `setup` creates a
pending secret, `enable` proves the app shows the right code and returns ten one-time recovery codes
(shown only once). These endpoints are refused for API tokens (`403`). OIDC logins are not asked for
a code (the identity provider is the second factor); API tokens are not affected.

| Method | Path                           | Description                                                                                                                                                                                               |
| ------ | ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`  | `/users/me/2fa`                | `{"enabled", "recovery_codes_remaining", "available"}`; `available` is false for accounts without a password (single sign-on only), which cannot use two-factor                                           |
| `POST` | `/users/me/2fa/setup`          | Create a pending secret: `{"secret", "otpauth_uri"}` (the URI is what the QR code encodes). `409` when already enabled, `400` for accounts without a password; calling it again replaces a pending secret |
| `POST` | `/users/me/2fa/enable`         | Body `{"password", "code"}`. Turns two-factor on, signs out the account's other sessions and returns `{"recovery_codes": [...]}`. `400` for a wrong password or code                                      |
| `POST` | `/users/me/2fa/disable`        | Body `{"password", "code"}` or `{"password", "recovery_code"}`. Removes the secret and the recovery codes and signs out the other sessions                                                                |
| `POST` | `/users/me/2fa/recovery-codes` | Same body as disable. Replaces the recovery codes and returns the new ones                                                                                                                                |

Login (`POST /login`) for an account with two-factor: the password alone answers
`200 {"totp_required": true}` and sets no cookie; repeat the request with `totp_code` (6 digits, one
use per 30-second step) or `recovery_code` (one use). A wrong code is `401` with code
`INVALID_TOTP_CODE`. Failed codes, and wrong passwords on the endpoints above, count toward the
account lockout (`429`); the counter is per account, not per identifier, and asking for the code
does not reset it.

### API Tokens

| Method   | Path              | Description                                           |
| -------- | ----------------- | ----------------------------------------------------- |
| `GET`    | `/api-tokens`     | List all API tokens for the current user              |
| `POST`   | `/api-tokens`     | Create an API token, returns the plaintext token once |
| `DELETE` | `/api-tokens/:id` | Revoke an API token                                   |

`POST /api-tokens` body:

```json
{ "name": "my token" }
```

Response includes `token` (the `meerkat_…` plaintext value) only on creation. Subsequent list
responses omit it.

### Calendar Feed (ICS)

Exports birthdays and reminders as a subscribable iCalendar feed. Each user has at most one feed,
protected by a random token that is part of the URL. Only a hash of the token is stored, so the URL
is shown once when it is created or regenerated.

| Method   | Path               | Auth                  | Description                                                                                                                                                                                        |
| -------- | ------------------ | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`    | `/ics-feed`        | session or API token  | Feed status: `{"enabled": bool, "created_at": ..., "last_fetched_at": ...}`. Never contains the token                                                                                              |
| `POST`   | `/ics-feed`        | session or API token  | Create the feed, or regenerate it (the old URL stops working). Returns `201` with `token` once, plus the status fields (`enabled`, `created_at`, `last_fetched_at`, which is `null` for a new URL) |
| `DELETE` | `/ics-feed`        | session or API token  | Revoke the feed (idempotent)                                                                                                                                                                       |
| `GET`    | `/ics/<token>.ics` | token in the URL only | The calendar (`text/calendar; charset=utf-8`)                                                                                                                                                      |

The feed URL is `<base>/api/v1/ics/<token>.ics`, where `<token>` is the 43 character `token` from
`POST /ics-feed`. `GET /ics/<token>.ics` takes no cookie or `Authorization` header, so calendar apps
can subscribe to it; it only ever returns the data of the token's owner. An unknown, malformed,
regenerated or revoked token answers `404`. The response carries `ETag` and
`Cache-Control: private, max-age=900` (conditional requests answer `304`), is rate limited per IP
like the other endpoints and is marked `noindex`.

Content:

- **Birthdays** of non-archived contacts as yearly all-day events (`RRULE:FREQ=YEARLY`). A birthday
  without a year (`--MM-DD`) starts in 1972; 29 February uses `BYMONTH=2;BYMONTHDAY=-1`.
- **Open reminders** as all-day events on their next date. `weekly`, `monthly`, `quarterly`,
  `six-months` and `yearly` recurrences become an `RRULE`; `once` reminders have none. Monthly-based
  rules for dates on the 29th to 31st use `BYMONTHDAY=28,..;BYSETPOS=-1` so short months get their
  last day instead of being skipped. Dates are UTC dates of `remind_at`. Completed reminders are
  left out.
- Event UIDs are stable (`contact-birthday-<id>@meerkat-crm`, `reminder-<id>@meerkat-crm`), and
  `DTSTAMP` is the last update time of the record.
- Event titles are in the language of the feed owner.

### Admin

| Method   | Path                         | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| -------- | ---------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`    | `/admin/system-status`       | Operational warnings: `{"private_urls_allowed", "user_count", "private_url_warning", "setting", "features"}`. `private_url_warning` is true when outbound features may reach private addresses and more than one user exists (fix: `BLOCK_PRIVATE_URLS=true`); `features` lists the features (`webhooks`, `caldav`, `carddav`, `monica`) that still allow private addresses. Shown as a banner on the Users page                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `GET`    | `/admin/audit-log`           | Security events, newest first: `{"entries": [{"id", "created_at", "user_id", "username", "event", "ip", "user_agent", "detail"}], "total", "page", "limit", "events", "retention_days"}`. Query: `event` (exact, one of `events`), `username` (substring), `page`, `limit`. Events: login, failed login, logout, password change/reset, session revoked, sign out everywhere, export, API token, calendar feed and backup actions, admin user changes and reminder runs, two-factor changes (enabled, disabled, recovery code used or regenerated, admin reset). `detail` is machine-readable, `;`-separated `key:value` segments (for example `reason:wrong_password`, `format:csv`, `user:alice#3;changed:email,password`) that the admin page translates. A failed login for an unknown account has no user and never stores what was typed. Entries older than `AUDIT_LOG_RETENTION_DAYS` (default 365, `0` = keep) are purged daily |
| `GET`    | `/admin/users`               | List all users                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `GET`    | `/admin/users/:id`           | Get a user                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `PATCH`  | `/admin/users/:id`           | Update a user (e.g. set admin flag)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `POST`   | `/admin/users/:id/2fa/reset` | Switch off a user's two-factor (lost phone and recovery codes) and sign the user out everywhere. `409` when the user has no two-factor (nothing changes). Audited. The user list and `GET /admin/users/:id` show `two_factor_enabled`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `DELETE` | `/admin/users/:id`           | Delete a user. Their data, API tokens, calendar feed, CardDAV connection, calendar subscriptions and webhooks are removed and their sessions end immediately (`401`). The account stays soft-deleted, but its email, username and OIDC identity are released: the person can register or log in via OIDC again and gets a new, empty account                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `GET`    | `/admin/backups`             | List the server backups, newest first, with the schedule: `{"backups": [{"name", "size", "created_at"}], "enabled", "time", "retention_count", "directory"}`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `POST`   | `/admin/backups`             | Create a backup now (`201`, `{"name", "size", "created_at"}`) and prune to `BACKUP_RETENTION_COUNT`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `GET`    | `/admin/backups/:name`       | Download a backup (`.tar.gz`, `Cache-Control: no-store`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `DELETE` | `/admin/backups/:name`       | Delete a backup                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `POST`   | `/admin/trigger-reminders`   | Send due reminder emails for all users now                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |

A backup contains the data of **all users** (every table, including password hashes) and all profile
photos, so these endpoints are for admins only and are refused for API tokens. `:name` must be
exactly a name returned by the list (`meerkat-backup-YYYYMMDD-HHMMSS.tar.gz`, optionally with a `-N`
suffix); anything else answers `404`. See [Backups](deployment.md#backups).

### Health

| Method | Path       | Description                                                                                                                                                                                                                                                                           |
| ------ | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`  | `/health`  | Health check (no auth, no versioning). Accessible directly (without prepending base URL).                                                                                                                                                                                             |
| `GET`  | `/metrics` | Prometheus metrics (text format). Only exists when `METRICS_ENABLED=true` and `METRICS_ADDR` is empty; with `METRICS_TOKEN` it needs `Authorization: Bearer <token>`. Not versioned and not proxied by the bundled nginx. See [Monitoring](deployment.md#monitoring-with-prometheus). |
