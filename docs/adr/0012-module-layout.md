---
title: '0012. Module layout (hexagonal)'
parent: ADRs
nav_order: 12
---

# 0012. Module layout (hexagonal)

- Status: Accepted
- Date: 2026-10-17

## Context

Controllers query GORM directly, so a business rule (such as "every query is scoped to the user") is
repeated in every handler, and it cannot be tested or reused by CardDAV, imports or jobs without
going through HTTP. [ADR 0010](0010-target-layering.md) stops wrong-way imports in the old layout;
[ADR 0011](0011-api-contract-with-huma.md) gives each converted endpoint a typed contract. This ADR
fixes the shape a converted module has. Contacts is the pilot; every other module follows the same
pattern.

## Decision

A module is a directory `backend/<module>/` with **one public entry package** and an `internal/`
tree that the Go compiler hides from everyone else:

```
backend/contacts/
  contacts.go                        public entry: New(Deps) *Module, (*Module).Register(...)
  internal/domain/                   entities and rules; standard library only
  internal/app/                      use cases and the ports they need
  internal/adapters/persistence/     GORM repositories, the unit of work (the only code that imports gorm)
  internal/adapters/http/            huma operations: typed input and output, map HTTP to use cases
  internal/adapters/webhooks/        implements the event port with services.TriggerWebhooks
```

- **Domain.** Entities and rules (`Contact`, archiving). No Gin, no GORM, no huma, no I/O, no clock,
  no `models`. A domain error such as `ErrNotFound` is part of the vocabulary.
- **Use cases.** One function per user intention (`ArchiveContact`, `DeleteContact`, `ListCircles`).
  Each takes the context, the actor (the user ID), the inputs and `now`; declares the persistence it
  needs as **ports** (small interfaces defined in `app`); and runs its writes inside one
  `Store.InTx`. Every query a port runs is scoped to the actor, so the isolation rule is written
  once, in the persistence adapter.
- **Adapters.** `persistence` implements the ports on top of the existing GORM models (the tables
  and `models.*` types do not move; the adapter maps them to domain entities). `http` registers huma
  operations, reads the actor and the clock from the request, calls a use case and maps the result
  and the errors (`domain.ErrNotFound` becomes `apperrors.ErrNotFound`) to the exact responses the
  Gin handlers sent before. Other entry points (CardDAV, jobs, imports) will call the same use
  cases.
- **Composition root.** `main.go` calls `routes.NewModules(db, cfg, oidcProvider)`, which builds
  each module from concrete values (`contacts.New(contacts.Deps{DB: db, Config: cfg})`) in one
  place; `routes` registers the operations on the Gin groups. `cmd/openapi` and the route tests call
  it too, with no database where only the spec is needed.
- **Entities versus tables.** The domain entity is the module's vocabulary; `models.Contact` stays
  the GORM model other, not yet converted code (CardDAV, sync, imports) still uses. Until those are
  converted the persistence adapter is the bridge, and `models` keeps the GORM hooks that maintain
  the denormalised columns.

**Enforced by `make arch`** (depguard, `backend/.golangci.yml`; each rule names the reason in its
message):

| Rule                               | Forbids                                                                                                       |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `domain-stays-pure`                | `internal/domain` importing anything but the standard library                                                 |
| `use-cases-depend-on-ports`        | `internal/app` importing gorm, gin, huma, `models`, `services`, `api` or any adapter                          |
| `repositories-stay-in-persistence` | gorm and `models` imported anywhere in the module except `internal/adapters/persistence` (and the entry seam) |
| modules through the entry package  | importing another module's `internal/` (the compiler)                                                         |

One named exemption: `contacts/contacts.go`, the entry package, imports gorm because `Deps.DB` is a
`*gorm.DB`. It is the seam between the composition root and the persistence adapter and does nothing
else with it.

## Plan for the pilot

Converting all contact endpoints in one commit is too much to review, so step 12 is delivered in
three parts, each green on its own and each leaving the old Gin handlers for the endpoints not yet
moved:

1. **12a: the pattern and the lifecycle endpoints.** The layout above, the ADR, the depguard rules
   and the composition root, with `archive`, `unarchive`, `DELETE /contacts/{id}` (to the trash,
   with the `contact.deleted` webhook) and `GET /contacts/circles` converted. This needs the typed
   `ContactResponse`, which later parts reuse.
2. **12b: read and write one contact.** `GET /contacts/{id}` (with `fields`), `POST /contacts`,
   `PUT /contacts/{id}`, the domain rules behind the denormalised columns, and the contact webhooks.
3. **12c: lists (done).** `GET /contacts` (search, FTS, circles, sort, paging, `fields`),
   `/contacts/random`, `/contacts/birthdays`; then delete the hand-written frontend contact types in
   favour of the generated ones, and delete the Gin handlers and `ValidateJSONMiddleware` use for
   contacts.

## Consequences

Business rules are testable with a real SQLite database through the use case, and through HTTP as
before. A new module costs a directory and a few ports, not a rewrite. Contacts, notes, activities,
reminders, relationships, photos, trash, users, tokens, webhooks, exports, imports and monicaimport
are converted; a route that sends bytes or redirects stays a Gin handler in its module's HTTP
adapter. Until the rest are, two styles coexist in the codebase; the route table in `routes` shows
which is which.
