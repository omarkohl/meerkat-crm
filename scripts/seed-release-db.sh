#!/usr/bin/env bash
# Builds backend/database/testdata/releases/<tag>.db with the migrations and the code of a release
# tag, then fills it with a few rows. Run it once per release you want to keep testing against.
# Usage: scripts/seed-release-db.sh v1.7.0   (needs git, Go and the network for old modules)
set -euo pipefail
tag="${1:?usage: seed-release-db.sh <git tag>}"
case "$tag" in */* | *..*) echo "not a tag name: $tag" >&2; exit 2 ;; esac
root="$(cd "$(dirname "$0")/.." && pwd)"
git -C "$root" rev-parse --verify --quiet "refs/tags/$tag^{commit}" >/dev/null || { echo "no such tag: $tag" >&2; exit 2; }
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

git -C "$root" archive "$tag" backend | tar -x -C "$work"
mkdir -p "$work/backend/cmd/seedrelease" "$root/backend/database/testdata/releases"
cp "$root/scripts/seed-release-db/main.go.txt" "$work/backend/cmd/seedrelease/main.go"
out="$root/backend/database/testdata/releases/$tag.db"
rm -f "$out" "$out-wal" "$out-shm"
# A clean environment: MEERKAT_* variables of the caller must not change what the old code does.
(cd "$work/backend" && env -i PATH="$PATH" HOME="$HOME" GOPATH="$(go env GOPATH)" GOCACHE="$(go env GOCACHE)" GOFLAGS="-mod=mod" go run ./cmd/seedrelease "$out")
echo "wrote $out"
