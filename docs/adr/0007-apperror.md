---
title: '0007. Errors are *apperrors.AppError'
parent: ADRs
nav_order: 7
---

# 0007. Errors are *apperrors.AppError

- Status: Accepted
- Date: 2026-10-17

## Context

The frontend needs stable error codes and the API needs one error format.

## Decision

Controllers return `*apperrors.AppError` with an HTTP status, a stable code and a message. The error
handler middleware at the top of the stack writes the structured JSON response and also catches
panics. Request bodies are validated with `api.ValidateBody` in the input's `Resolve` method
([ADR 0011](0011-api-contract-with-huma.md)).

## Consequences

One error format for every endpoint. Codes are part of the API contract.
