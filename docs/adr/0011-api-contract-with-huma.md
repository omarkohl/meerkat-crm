---
title: '0011. API contract with huma'
parent: ADRs
nav_order: 11
---

# 0011. API contract with huma

- Status: Accepted
- Date: 2026-10-17

## Context

The API is described by hand in `docs/api-reference.md` and by hand-written TypeScript types, so the
two drift from the code. The backend is converted to a hexagonal layout module by module, and every
converted endpoint should come with a machine-checked contract.

## Decision

Endpoints are converted one by one to [huma](https://huma.rocks) operations (code-first: Go input
and output structs are the contract). The package `backend/api` does the wiring:

- **Mounted on Gin, not replacing it.** `api.New(group, engine)` registers operations on an existing
  Gin group, so the group's middleware (request ID, clock, auth, rate limits) runs around them.
  Per-route middleware is passed to `api.Register(..., ginMiddleware...)`. No docs, schema or spec
  routes are served.
- **The AppError format is kept.** Errors raised by huma (validation, malformed JSON) and
  `*apperrors.AppError` returned by handlers are written as
  `{"error": {"code", "message", "details"}, "request_id", "timestamp"}`. Validation failures stay
  `400 VALIDATION_ERROR` with `details` keyed by field (huma's own default is 422), malformed JSON
  is `400 INVALID_INPUT`. Handlers return `apperrors.ErrNotFound(...)` and friends as before.
- **The six custom validators work unchanged.** `api.ValidateBody` runs the existing go-playground
  validator (`phone`, `birthday`, `strong_password`, `unique_circles`, `no_at_sign`, `safeurl`) from
  the input struct's `Resolve` method. The old `ValidateJSONMiddleware` is gone; the detail key is
  the JSON field name.
- **Spec and types are generated and committed.** `make openapi` writes `docs/openapi.json` (OpenAPI
  3.1, by `backend/cmd/openapi`, which needs no database) and `frontend/src/types/generated/api.ts`
  (by `openapi-typescript`). `make openapi-check` regenerates both into a temporary directory and
  fails on any difference; it runs in `make precommit` and in CI.
- **First operation:** `GET /auth/oidc/config`; the login hook uses the generated response type.

**Request bodies behave as `ShouldBindJSON` plus the `validate` tags did** (settled before the first
real conversion, `api.relaxRequestBody`, run by `api.Register`):

- Unknown JSON properties are ignored, not rejected (huma rejects them by default).
- A property is required when its `validate` tag says `required` (or it has `required:"true"`), not
  because its JSON tag lacks `omitempty`. The spec lists exactly those, and huma's own "expected
  required property" errors are dropped, so a missing field is reported once, in the project's
  wording (`email is required`).
- Detail keys are JSON names, also for nested items (`email`, `phones[0].value`), where the old
  middleware used Go field names (`Email`, `Value`). The frontend reads only the values of
  `details`, so nothing depends on the keys. Old endpoints keep the old keys until converted
  (`middleware.ValidateStruct` vs `ValidateStructJSON`; same rules, same messages).
- An empty or non-JSON body is `400 INVALID_INPUT`; a body over the size limit is
  `413 INVALID_INPUT`; a wrong media type is `415 INVALID_INPUT`. The limit is the one the global
  body-size middleware already enforces (`middleware.DefaultMaxBodySize`, 10 MiB), set explicitly as
  `MaxBodyBytes`.
- Errors are logged like `apperrors.RespondWithError` did (`apperrors.LogError` with the request
  logger); the cause of a 500 is logged and never sent.

Things to know when converting an endpoint:

- Shared schemas: the request-body normalisation changes the schema of the body type, so a type used
  both as request body and as response loses huma's response-side `required` and gets no
  `additionalProperties: false`. Use separate input and output types (the hexagonal layout does
  anyway).
- Responses differ from gin's `c.JSON` in bytes, which is accepted: a trailing newline, no
  `\u003c`-style HTML escaping (`<`, `>`, `&` are written as they are), and
  `Content-Type: application/json` without a charset. Clients parse JSON, so none of this matters to
  them.
- huma does not add a `$schema` field to responses here: the default configuration is not used.
- The error schema `Error` in the spec is the AppError response, whatever status the operation
  documents.
- `cmd/openapi` registers operations with an empty config, so an operation inside an `if cfg.X`
  would be missing from the spec. Register every operation unconditionally; a feature that is off
  answers as before (the OIDC login routes that exist only when OIDC is on are not huma operations
  yet and will need this rule).
- Operations get their database and config through closures over the values `routes` already has;
  `routes.RegisterAPI` takes only what registration needs, which keeps spec generation free of a
  database.

## Consequences

`docs/openapi.json` replaces the hand-written route tables step by step (every JSON route is an
operation; `routes/contract_test.go` fails otherwise, and lists the routes that send files or
redirect). Hand-written frontend types for converted endpoints are deleted in favour of the
generated ones. A change to an operation needs `make openapi` in the same commit.
