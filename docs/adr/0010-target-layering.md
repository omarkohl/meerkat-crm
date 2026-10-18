---
title: '0010. Target layering'
parent: ADRs
nav_order: 10
---

# 0010. Target layering

- Status: Accepted
- Date: 2026-10-17

## Context

Controllers query GORM directly and nothing stops a package from reaching up the stack. The run
towards a hexagonal backend ([ADR 0012](0012-module-layout.md)) needs a guard first: a wrong-way
import must fail the build, so each later step can only improve the picture.

## Decision

**Backend.** Dependencies point down: `main.go` and `routes` > `controllers` > `services` >
`models`. `depguard` (in `backend/.golangci.yml`, run by `make arch` and `make lint`) fails the
build when:

- `models` imports `controllers`, `services` or `gin`;
- `services` imports `controllers` or `gin`;
- anything but `routes`, `controllers` itself and the root `backend/main.go` imports `controllers`.

The rules check direct imports only and include test files. Today there is no Go violation and so no
Go exemption (`services/reminder_service_test.go` had an unused gin router; it was removed).
`services` still imports `middleware` in one file; that stays allowed until the layering ADR for the
pilot module ([ADR 0012](0012-module-layout.md)) decides where `middleware` belongs.

**Frontend.** Dependencies point down: pages (`src/*Page.tsx`) > hooks > `src/api` resource
modules > `src/api/client.ts`. `dependency-cruiser` (`frontend/.dependency-cruiser.cjs`, run by
`make arch`) fails on a circular import, an unresolvable import, a page that imports `src/api`, any
import of `src/api/client.ts` from outside `src/api`, and a re-export of `client.ts` from the
`src/api/index.ts` barrel (which would bypass the previous rule). The frontend stays simple: no
hexagonal layers.

**Baseline.** Violations that existed when the rules were added are recorded per pair in
`frontend/.dependency-cruiser-known-violations.json` (30 pairs). A new violation fails; a recorded
one that was fixed fails too until it is removed (`node scripts/arch-frontend.mjs --update`), so the
file only shrinks. Reasons, by rule:

- `pages-use-api-through-hooks` (24 pairs): pages predate the hook layer and call `src/api` for
  writes and one-off reads. Fix by moving the call into a hook in `src/hooks`.
- `api-client-is-private-to-api` (6 pairs): components and `utils/errorHandler.ts` use `ApiError`
  and `ApiClient` helpers from `client.ts`. Fix by re-exporting what they need from a resource
  module or `src/api/index.ts`.

Two former cycles were fixed instead of recorded: `types/api.ts` and `types/forms.ts` no longer
import `types/index.ts` (`User` and `Relationship` live in `types/entities.ts`).
`src/react-app-env.d.ts` is excluded from the cruise: it only references the `react-scripts` type
package.

## Update (2026-10-18)

The Go layering above is the baseline. Every converted module adds its own `depguard` rules in
`backend/.golangci.yml` (`<module>-domain-stays-pure`, `-use-cases-depend-on-ports`,
`-repositories-stay-in-persistence`, [ADR 0012](0012-module-layout.md)); `controllers` now holds
only the health check and the image proxy. The frontend baseline file is the current state of the
recorded pairs, not the 30 of the day it was introduced.

## Consequences

Each hexagonal step tightens the Go rules for the module it migrates. Each frontend fix removes
pairs from the baseline. A rule that proves wrong is changed here first.
