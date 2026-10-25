# Fork: branches, versions and releases

This fork of [fbuchner/meerkat-crm](https://github.com/fbuchner/meerkat-crm) publishes its own
images to `ghcr.io/omarkohl/meerkat-crm{,-backend,-frontend}`.

See [DIFFERENCES.md](../../DIFFERENCES.md) for what the fork changes compared to upstream.

## Branches

- `main` tracks upstream `main`. It has no fork changes.
- `omk` has the fork changes, the ported upstream commits and merge records of `main`. It is never
  rebased. Fork releases come only from `omk`. Set `omk` as the default branch on GitHub, so
  scheduled workflows and Dependabot use it.

## Versions

`vX.Y.Z-omk.N`: `X.Y.Z` is the newest upstream tag in the history of `omk` (through a merge record),
`N` counts fork releases on that base (e.g. `v1.7.0-omk.1`, `v1.7.0-omk.2`, `v1.8.0-omk.1`). Fork
tags never collide with upstream tags. There is no `latest` or `X.Y` image tag.

## Integrate upstream

Port upstream commits one at a time, then record a merge that holds no port work
([ADR 0020](../adr/0020-upstream-integration.md)).
[`upstream-ledger.yml`](../../upstream-ledger.yml) records each upstream commit and has the format.

1. Fetch and list the new upstream commits: after the newest hash in the ledger, or after `merged`
   when the list is empty. Add each one to the ledger as `pending`.

   ```sh
   jj git fetch --remote upstream
   jj bookmark set main -r main@upstream
   jj log --reversed --no-graph -r '<hash>..main ~ merges()' \
     -T 'commit_id.short(12) ++ " " ++ description.first_line() ++ "\n"'
   ```

2. For each `pending` entry, oldest first, do one of these:
   - Port it: a fork commit with the adapted change, its tests, the footer `Upstream: <hash>` and
     the ledger entry set to `ported`. A port of several upstream commits has one footer for each.
   - Set it to `skipped` or `superseded` with a `reason`, in a `chore:` commit or in the next port.

   Each commit passes `make precommit`. Rules for a port:
   - **One topic, one port.** When several upstream commits rework one topic, port the final state
     (`git show X:<path>`, `git diff <merged> X -- <paths>`) in one commit. Upstream merge commits
     can hold conflict fixes that only the final state shows.
   - **Upstream author.** After `jj commit`, set it with
     `jj metaedit -r @- --author "Name <email>"`. With several authors, the author is the person
     with most of the covered commits, and each other author gets a `Co-authored-by:` trailer.
   - **Migrations.** An upstream migration becomes a new fork migration with the next free number
     and the same SQL. From 000024 on, the fork's numbers differ from upstream's. Never convert an
     upstream database with upstream's down migrations: they can lose data.
   - **Superseded.** Upstream took a fork PR, possibly only in part, or the fork has the same or a
     newer dependency. Check it in the code, `backend/go.mod` or `frontend/package-lock.json`, and
     reduce [DIFFERENCES.md](../../DIFFERENCES.md) to what upstream does not have.

3. Record the merge through upstream commit `X` (usually a tag) when no entry up to `X` is
   `pending`:

   ```sh
   jj new omk X -m "chore: merge upstream through X"
   jj restore --from omk
   # set `merged: X` in upstream-ledger.yml
   make precommit
   jj bookmark set omk -r @
   jj new
   jj git push -b main -b omk
   ```

   `jj restore` makes the tree of the merge the same as `omk`. The merge only tells Git that
   upstream up to `X` is integrated.

## Release

First rename `[Unreleased]` in `CHANGELOG.md` to the version and date, add the compare link, and
commit it to `omk`. The release notes are that section.

```sh
git tag v1.8.0-omk.1 <commit on omk>
git push origin v1.8.0-omk.1
```

The tag push runs `.github/workflows/docker-publish.yml`. It fails if the tagged commit is not on
`omk`.
