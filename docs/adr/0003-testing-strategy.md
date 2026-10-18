---
title: '0003. Testing strategy'
parent: ADRs
nav_order: 3
---

# 0003. Testing strategy

- Status: Accepted
- Date: 2026-10-17

## Context

A test must fail when a user's outcome changes, and ideally only then. Tests that mirror the
implementation break on every refactor and catch few real bugs.

## Decision

Tests have three tiers.

- **Integration** (the bulk): business logic through its real entry points (HTTP handlers, use
  cases) against a real SQLite database. Mock only what cannot run locally (Resend, remote OIDC).
  Check results through a read surface: the matching read endpoint first, then repository reads, raw
  rows last. Scheduler jobs do not run implicitly in tests; tests trigger them.
- **E2E** (few): important user workflows in Playwright. Act like a user: role, label and
  visible-text locators, no CSS or test IDs, no URLs built from internal IDs. Tests that change
  global settings run serially after the parallel ones. CI runs the suite against the all-in-one
  Docker image.
- **Unit** (rare): complex isolated functions only (parsers, ICS folding). Never repeat integration
  coverage.

Guardrails: a test that breaks on an internal rename without a visible change is rewritten or
deleted. No assertions on call counts of internal helpers. No shared state between tests. Factories
build minimal valid entities. Coverage thresholds (Vitest and Go) stop slow erosion;
`make test-coverage` is part of `make precommit` and CI. Invariant tests enumerate the Gin routes
and the models with a `user_id` and fail when one has no auth check or isolation test
(`backend/routes/invariants_test.go`). The use-case catalogue `docs/use-cases.yml` gives every test
a permanent ID (`make use-cases`). Upgrade tests migrate databases made by past releases
(`backend/database/testdata/releases/`), and the Radicale scenarios fail instead of skipping on CI
when the service is not configured.

TDD is mandatory for agents: write the failing test, run it and confirm the failure, implement the
minimum, confirm green, refactor without touching the test. Exceptions: pure styling changes, and
refactors that tests already cover fully.

## Consequences

Refactors stay cheap and the suite stays fast. Details and commands live in
[`docs/development/testing.md`](../development/testing.md).
