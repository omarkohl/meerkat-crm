---
title: '0016. Changelog and commit rules'
parent: ADRs
nav_order: 16
---

# 0016. Changelog and commit rules

- Status: Accepted
- Date: 2026-10-18

## Context

The changelog grew into a second commit history: long, technical entries with rationale and file
names, which users and admins do not read. Commit subjects varied in form and length, so the
changelog and the release notes could not be derived from them.

## Decision

- Commits use conventional prefixes and a subject of at most 72 characters; the body says why. A CI
  job on pull requests checks the subjects (`scripts/check-commits.mjs`).
- `CHANGELOG.md` is for the users and admins of the CRM. One bullet per change, a bold lead phrase,
  `(#123)` only when a commit footer names the issue, at most about 40 words, no sub-bullets.
  `Internal` lists only valuable or disruptive internal changes, at most three per release, 25 words
  each. `scripts/check-changelog.mjs` checks the `[Unreleased]` section in `make lint` and CI. The
  exact rules are in [AGENTS.md](../../AGENTS.md).
- Fork releases are written from the commits since the previous release; upstream releases from the
  upstream release notes.

## Consequences

A long or technical entry fails the lint and has to be shortened. The script cannot tell whether an
issue number matches a commit footer, or whether an entry is non-technical: review checks that. A
history that predates this ADR (released sections) is not rewritten or checked.
