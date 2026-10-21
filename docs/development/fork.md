# Fork: branches, versions and releases

This fork of [fbuchner/meerkat-crm](https://github.com/fbuchner/meerkat-crm) publishes its own
images to `ghcr.io/omarkohl/meerkat-crm{,-backend,-frontend}`.

See [DIFFERENCES.md](../../DIFFERENCES.md) for what the fork changes compared to upstream.

## Branches

- `main` tracks upstream `main`. It has no fork changes.
- `omk` is `main` plus the fork changes. Fork releases come only from `omk`. Set `omk` as the
  default branch on GitHub, so scheduled workflows and Dependabot use it.

## Versions

`vX.Y.Z-omk.N`: `X.Y.Z` is the upstream tag the release is based on, `N` counts fork releases on
that base (e.g. `v1.7.0-omk.1`, `v1.7.0-omk.2`, `v1.8.0-omk.1`). Fork tags never collide with
upstream tags. There is no `latest` or `X.Y` image tag.

## Integrate upstream

```sh
jj git fetch --remote upstream
jj bookmark set main -r main@upstream
jj rebase -b omk -d 'tags(exact:"v1.8.0")'
jj git push -b main -b omk
```

## Release

First rename `[Unreleased]` in `CHANGELOG.md` to the version and date, add the compare link, and
commit it to `omk`. The release notes are that section.

```sh
git tag v1.8.0-omk.1 <commit on omk>
git push origin v1.8.0-omk.1
```

The tag push runs `.github/workflows/docker-publish.yml`. It fails if the tagged commit is not on
`omk`.
