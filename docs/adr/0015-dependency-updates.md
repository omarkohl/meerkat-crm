---
title: '0015. Dependency updates'
parent: ADRs
nav_order: 15
---

# 0015. Dependency updates

- Status: Accepted
- Date: 2026-10-18

## Context

Dependabot proposed one pull request per package and week, majors and patches mixed. Brand-new
releases are where supply-chain attacks and broken releases land. Floating Docker tags such as
`golang:1.27-alpine` hide changes from Dependabot, and an `ignore` rule without a reason is never
removed.

## Decision

- Dependabot checks **daily** with `cooldown: {default-days: 7}`: a release is proposed only after
  it has been out for a week.
- **One group per ecosystem** (Go, npm, Actions, Docker, Bundler) with all update types, so there is
  one pull request per ecosystem to review and one CI run.
- A major update that breaks the build is held back with an `ignore` entry. **Every `ignore` has a
  comment** with what blocks it, the check that shows it is unblocked, and the issue where one
  exists. An entry for which no issue is known says so; it never gets an invented number.
- Docker base images are pinned to the **full version of the program**, in one form for all of them:
  `<image>:<x.y.z>-alpine` for Go, Node and nginx, and `alpine:<x.y.z>` for the runtime. The Alpine
  release floats with the tag, so a new Alpine reaches the builders and the runtime through the same
  Dependabot pull request. GitHub Actions stay pinned to commit SHAs.

## Consequences

Updates arrive as a few larger pull requests, a week after release. A breaking major shows up in the
group's pull request until someone adds an `ignore` entry with its reason. The list of `ignore`
entries is the list of known upgrade debts. Because the builder images float on the Alpine release,
a rebuild can pick up a newer Alpine without a version change: the pin protects against a new
program version, not against Alpine updates.
