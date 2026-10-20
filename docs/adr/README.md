---
title: ADRs
nav_order: 10
has_children: true
---

# Architecture decision records

One file per significant decision, see [0001](0001-record-decisions.md). Never delete an ADR:
supersede it.

| ADR                                         | Title                                      | Status   |
| ------------------------------------------- | ------------------------------------------ | -------- |
| [0001](0001-record-decisions.md)            | Record decisions as ADRs                   | Accepted |
| [0002](0002-rules-are-machine-checks.md)    | Rules are machine checks with fix messages | Accepted |
| [0003](0003-testing-strategy.md)            | Testing strategy                           | Accepted |
| [0004](0004-sqlite-and-few-moving-parts.md) | SQLite with WAL and few moving parts       | Accepted |
| [0005](0005-user-id-isolation.md)           | Every query is scoped by user_id           | Accepted |
| [0006](0006-never-edit-a-migration.md)      | Never edit an existing migration           | Accepted |
| [0007](0007-apperror.md)                    | Errors are \*apperrors.AppError            | Accepted |
| [0008](0008-five-locales.md)                | Five locales for every UI string           | Accepted |
| [0009](0009-injected-clock.md)              | Injected clock                             | Accepted |
| [0010](0010-target-layering.md)             | Target layering                            | Accepted |
| [0011](0011-api-contract-with-huma.md)      | API contract with huma                     | Accepted |
| [0012](0012-module-layout.md)               | Module layout (hexagonal)                  | Accepted |
| [0013](0013-vite.md)                        | Vite builds the frontend                   | Accepted |
| [0014](0014-npm.md)                         | npm manages frontend dependencies          | Accepted |
| [0015](0015-dependency-updates.md)          | Dependency updates                         | Accepted |
| [0016](0016-changelog-and-commit-rules.md)  | Changelog and commit rules                 | Accepted |
| [0017](0017-fetching-user-supplied-urls.md) | Fetching user-supplied image URLs          | Accepted |

New ADR: copy a recent one, take the next number, add a row here.
