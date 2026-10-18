# Meerkat CRM – High-Value Improvements

Open items only; finished items were removed (numbers stay stable). Ordered by value within each
section. Effort: **S** (< 1 day), **M** (days), **L** (week+).

## 2. Testing

| #    | Item                                                                                          | Effort | Why                                                         |
| ---- | --------------------------------------------------------------------------------------------- | ------ | ----------------------------------------------------------- |
| 2.9  | Playwright: add a11y checks (`@axe-core/playwright`) and visual snapshots for dark/light mode | S      | Low cost, catches regressions in themes.                    |
| 2.10 | Contract tests: generate OpenAPI (see 4.1) and validate responses in Go tests                 | M      | Frontend `types/api.ts` and backend structs drift silently. |

## 3. CI/CD & Supply Chain

| #   | Item                                                                                        | Effort | Why                   |
| --- | ------------------------------------------------------------------------------------------- | ------ | --------------------- |
| 3.6 | Release automation (`release-please` or `goreleaser`) with conventional commits → changelog | M      | Manual tagging today. |

## 4. Architecture & Code Structure

| #    | Item                                                                                                                                                 | Effort | Why                                                                                                                                                                       |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 4.1  | **OpenAPI spec** (code-first with `swaggo/swag` or `huma`, or spec-first with `oapi-codegen`) + generated TS client (`openapi-typescript`)           | L      | ~100 routes, hand-written `docs/api-reference.md` and hand-written `frontend/src/api/*.ts`. One source of truth removes a whole bug class and gives API users Swagger UI. |
| 4.2  | Repository layer between controllers and GORM                                                                                                        | M      | Controllers contain query logic (e.g. search `LIKE` clauses). Easier testing.                                                                                             |
| 4.3  | Split large files: `contact_sync_service.go` (1.3k LOC), `monica_import_session.go` (1.2k), `import_service.go` (1k), `ContactDetailPage.tsx` (800)  | M      | Readability and review cost.                                                                                                                                              |
| 4.4  | Frontend data layer: **TanStack Query** instead of custom `{data, loading, error, refetch}` hooks                                                    | M      | Caching, dedup, optimistic updates, background refetch for free. Removes most of `hooks/`.                                                                                |
| 4.5  | Form library (`react-hook-form` + `zod`) for large dialogs                                                                                           | M      | Shared validation; schemas can be generated from OpenAPI.                                                                                                                 |
| 4.6  | Route-level code splitting (`React.lazy`) – especially `NetworkPage` (force-graph) and import dialogs                                                | S      | Smaller initial bundle.                                                                                                                                                   |
| 4.7  | Typed config via `caarlos0/env` or `kelseyhightower/envconfig`                                                                                       | S      | Replaces manual env parsing in `config.go`.                                                                                                                               |
| 4.8  | Fix stale docs: `README-developer.md` links `backend/middleware/middleware.go` (does not exist → `errors/middleware.go`); mixes `npm run` and `yarn` | S      | Onboarding friction.                                                                                                                                                      |
| 4.9  | Add `CONTEXT.md` (domain glossary: contact, circle, activity, note, reminder, connection) and `docs/adr/`                                            | S      | Helps contributors and AI agents.                                                                                                                                         |
| 4.10 | Remove legacy compose files (`docker-compose.legacy.yml`, `docker-compose.build.legacy.yml`) or move to `deploy/legacy/`                             | S      | Root clutter.                                                                                                                                                             |

## 6. Dependencies

| #   | Item                                                                                 | Effort | Why                                                                                                                |
| --- | ------------------------------------------------------------------------------------ | ------ | ------------------------------------------------------------------------------------------------------------------ |
| 6.5 | `@vitejs/plugin-react` ^4 → current; `jsdom` ^26 → current; Vitest 3 → 4             | S      | Stay in step.                                                                                                      |
| 6.6 | Evaluate `glebarez/sqlite` (pure Go, wraps modernc) vs `ncruces/go-sqlite3` (wazero) | S      | Both pure-Go; `ncruces` tracks SQLite upstream faster and supports FTS5 well. `wazero` is already an indirect dep. |

## 7. Security

| #   | Item                                                                     | Effort | Why                                                                             |
| --- | ------------------------------------------------------------------------ | ------ | ------------------------------------------------------------------------------- |
| 7.1 | **Passkeys** (WebAuthn, `go-webauthn/webauthn`); TOTP exists             | M      | CRM of personal data; TOTP is optional; passkeys give phishing-resistant login. |
| 7.2 | Encryption at rest option for notes (per-user key derived from password) | L      | Differentiator for privacy-focused self-hosters.                                |

## 8. Features

### Relationship intelligence

- **Keep-in-touch cadence per contact/circle** ("every 30 days") with an auto-computed _"overdue"_
  list on the dashboard, based on last activity. Turns the CRM from passive storage into an active
  nudge. **M**
- **Relationship health score**: frequency + recency of activities; sparkline on contact card. **M**
- **"On this day"** dashboard widget: activities and notes from 1/5/10 years ago. **S**
- **Gift ideas & wishlist** per contact, surfaced N days before birthday reminder. **S**
- **Important dates** beyond birthdays (anniversaries, name days, death dates) with reminders. **S**

### Capture & input

- **Quick-capture command palette** (`Ctrl+K`): search contacts, "log call with Anna", jump to
  pages. **M**
- **Markdown in notes** (safe renderer like `react-markdown` + `rehype-sanitize`) and `@mentions` of
  contacts that create links. **M**
- **PWA share target**: share text/URL/photo from phone into a note for a contact. Service worker
  already exists. **S**
- **Email/Telegram/Matrix bot ingestion**: forward a message → becomes a note on the matching
  contact. **L**
- **Attachments** on notes/activities (files, not only profile photos). **M**

### Notifications & integrations

- **ntfy / Gotify / Apprise / Web Push** notifications in addition to email. Apprise covers ~100
  services in one integration. **S–M**
- **Google Contacts / Outlook import** via CSV presets or OAuth. **M**
- **Home Assistant / n8n** friendly: document webhooks + API tokens with examples. **S**

### Organization & UX

- **Saved filters / smart circles** (e.g. "Berlin + no contact in 90 days"). **M**
- **Map view** of contacts by address (Leaflet + OSM, optional geocoding). **M**
- **Duplicate detection & merge** (name/email/phone similarity) — high value after CSV/VCF/Monica
  imports. **M**
- **Timeline filters and export** (PDF/Markdown of a contact's history). **S**
- **Keyboard shortcuts** throughout (`n` new note, `a` activity, `/` search). **S**

### Optional AI (opt-in, self-host friendly)

- Pluggable LLM provider (Claude API or local Ollama) for: summarizing a contact's history before a
  meeting, extracting dates/facts from free-text notes into structured fields, suggesting
  follow-ups. Off by default; never sends data without explicit config. **M**

### Multi-user

- **Shared circles** between users of one instance (e.g. household shares "Family"). **L**

## 9. Documentation & Community

- Architecture diagram (request flow nginx → Gin → GORM/SQLite, scheduler, DAV sync) in
  `docs/development/architecture.md`. **S**
- `CONTRIBUTING.md` at root (GitHub surfaces it), `SECURITY.md` with disclosure process,
  `CODE_OF_CONDUCT.md`. **S**
- PR template with checklist (tests, i18n keys for all 5 locales, migration up/down). **S**
- Helm chart / Unraid / TrueNAS / Umbrel app templates for discoverability. **M**
- Translation platform (Weblate free for OSS) to grow beyond 5 languages. **S**
