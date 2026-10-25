---
title: '0020. Integrate upstream: port, ledger, empty merge'
parent: ADRs
nav_order: 20
---

# 0020. Integrate upstream: port, ledger, empty merge

- Status: Accepted
- Date: 2026-10-25

## Context

The fork differs from upstream by about 94k added and 27k removed lines, with a whole-repo reformat
and moved modules ([DIFFERENCES.md](../../DIFFERENCES.md)). We want to keep upstream changes. The
fork may move closer to upstream or make a clean break later.

- A rebase of `omk` on each upstream tag solves conflicts once per fork commit, and the work grows
  with the fork history. It also rewrites published commits and release tags.
- A merge that holds the conflict work hides a large, unreviewed change in one commit.
- A patch queue (Debian, Electron) fits a fork that is small compared to upstream. This fork is not.
- Ports alone (Neovim's `vim-patch`, Forgejo after its hard fork) give no merge base, so Git cannot
  show which upstream commits are new.

## Decision

- `main` mirrors upstream `main`. It has no fork commits.
- Each upstream commit is ported as an ordinary fork commit with the footer `Upstream: <hash>`, or
  skipped, or marked as superseded. Each port passes `make precommit`. Several upstream commits on
  one topic become one port of their final state. A port keeps the upstream author.
- An upstream migration gets the next free fork number. Upstream databases are not converted.
- [`upstream-ledger.yml`](../../upstream-ledger.yml) records the status of each upstream commit.
  Scripts can read it.
- When no entry up to an upstream commit X is pending, a merge commit with parents `omk` and X
  records the integration. Its tree is the tree of `omk` plus the ledger change: it holds no port
  work.

## Consequences

Port work is reviewed and tested commit by commit, and history is never rewritten. Git uses X as the
merge base, so it shows only newer upstream commits. Git does not offer a commit again after the
merge record: an entry in the ledger with the wrong status is not caught. Blame shows the upstream
author, and the footer links to the upstream commit. Intermediate upstream commits of a topic are
not in the fork history. The migration numbers of the fork and upstream differ, so the fork cannot
open a database that upstream migrated past 000023. On a clean break, merge records stop and the
ledger continues.
