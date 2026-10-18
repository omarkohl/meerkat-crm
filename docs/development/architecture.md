---
title: Architecture
parent: Development
nav_order: 1
---

# Architecture

## Overview

Two codebases: a Go API (`backend/`) and a React SPA (`frontend/`). They communicate over HTTP, the
frontend calls `/api/v1/*` endpoints. In production they are bundled into a single all-in-one Docker
image where nginx serves the SPA and proxies `/api/` to the backend on `127.0.0.1:8080`; only the
nginx port is published.

Significant decisions are recorded as [ADRs](../adr/README.md).

## Backend

### Project Structure

```
backend/
  main.go              # Init: logger, config, DB, scheduler, router, graceful shutdown
  config/              # Environment variable loading and validation
  routes/routes.go     # All route registrations
  middleware/          # Auth, rate limiting, validation, logging, request ID
  controllers/         # the health check and the image proxy; everything else is in the modules
  api/                 # huma wiring: operations on Gin groups, AppError format (ADR 0011)
  contacts/            # first hexagonal module (all contact endpoints): domain, app, adapters (ADR 0012)
  notes/               # hexagonal module for notes, same layout
  activities/          # hexagonal module for activities, same layout
  relationships/       # hexagonal module for relationships and the network graph, same layout
  photos/              # hexagonal module for profile pictures and thumbnails (image downloads are plain Gin handlers)
  trash/               # hexagonal module for the trash; restore and purge still run in services (shared with the contact sync)
  tokens/              # hexagonal module for API tokens and the calendar feed (the feed itself is a plain Gin handler)
  webhooks/            # hexagonal module for managing webhooks; delivery and retries still run in services (every module's events share them)
  exports/             # hexagonal module for the CSV and vCard downloads (plain Gin handlers: they send files)
  imports/             # hexagonal module for the CSV and vCard import wizard (the parsing, mapping and merging stay in services, shared with the Monica import)
  monicaimport/        # hexagonal module for the import from Monica (the fetch, mapping and applying stay in services)
  calendars/           # hexagonal module for calendar subscriptions (the sync engine and its job stay in services)
  admin/               # hexagonal module for the admin endpoints: user management, backups, the audit log, the system status, the reminder trigger (the backup engine stays in services; the download is a plain Gin handler)
  contactsync/         # hexagonal module for the CardDAV connection settings (the sync engine and the protocol endpoints are elsewhere)
  users/               # hexagonal module for user preferences, sessions, passwords, two-factor, registration, login and logout, single sign-on (redirects are plain Gin handlers)
  reminders/           # hexagonal module for reminders, same layout, plus the daily reminder e-mail job (`SendDailyDigest`; the lock around it and the e-mail outbox are in services)
  models/              # GORM models
  services/            # Logic the modules still share: sync engines, webhook delivery, e-mail outbox, import parsing, backups, jobs
  cmd/openapi/         # Writes docs/openapi.json (make openapi)
  cmd/migrate/         # Runs migrations by hand (cd backend && make migrate-up)
  search/ session/ netguard/ clock/ logger/ idutil/ httputil/ photofile/ monica/  # small shared packages (the tree shows the main parts only)
  errors/              # AppError type and error handler middleware
  database/migrations/ # Embedded SQL migrations, auto-applied on startup
  carddav/             # CardDAV protocol implementation
  i18n/                # Backend translations (email notifications)
```

### Error Handling

Handlers (the huma operations in each module's `adapters/http`) return `*apperrors.AppError`; the
AppError format is the error body of every operation. The error handler middleware catches the rest
and unhandled panics ([ADR 0007](../adr/0007-apperror.md)).

```go
return nil, apperrors.ErrNotFound("Contact")
```

### Database Layer

SQLite via GORM. All SQL migrations live in `database/migrations/` as embedded files and run
automatically on startup in version order (`cd backend && make migrate-create NAME=xxx` adds a new
pair, see [Backend](backend.md#database-migrations)).

Every table includes `user_id` for multi-tenant isolation and all queries must filter by it.

## Frontend

### Project Structure

```
frontend/src/
  api/          # One module per resource (contacts, activities, …) + client.ts
  hooks/        # Data-fetching hooks (useContacts, useActivities, …)
  components/   # Reusable dialog and display components
  *Page.tsx     # Top-level route components (one per page), next to App.tsx
  types/        # Shared TypeScript types; types/generated/api.ts is generated (make openapi)
  i18n/locales/ # en, de, it, es, fr
```

### State Management

No global store. Each page owns its state via custom hooks. Hooks encapsulate API calls and expose
`{ data, loading, error, refetch }`. Dialogs manage local open/close state in `useContactDialogs`
and similar hooks.

### Routing

React Router. Each page component maps to a route. Protected routes redirect to `/login` on 401
(handled automatically in `api/client.ts`).

### Internationalization

`i18next` with `react-i18next`. All UI strings live in `i18n/locales/` (en, de, it, es, fr; see
[ADR 0008](../adr/0008-five-locales.md)). Use the `useTranslation()` hook. The user's language
preference is stored server-side and applied to backend email notifications too.

## Security

**Authentication** — JWT (HMAC-signed). Tokens are stored in an `httpOnly` cookie for the browser;
API clients (e.g. CardDAV) use a `Bearer` header fallback. Tokens carry `user_id` and `username`
claims and expire automatically (expiration set by env variable).

**Multi-tenant isolation** — every table has a `user_id` column; all queries filter by the ID
extracted from the validated JWT, so one user cannot access another's data.

**Password policy** — entropy-based validation (minimum 50 bits). Passwords are rejected at
registration/change time before they are hashed.

**Account lockout** — after 5 failed login attempts the account is locked with exponential back-off
(1 min base, doubling up to 30 min). The lockout state is reset after 1 hour of inactivity.

**Rate limiting** — per-IP token-bucket limiter applied globally; tighter limits on auth endpoints.

**Request size limits** — JSON bodies capped at 1 MB, other bodies at 10 MB to prevent DoS via large
payloads.
