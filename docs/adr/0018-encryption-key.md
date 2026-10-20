---
title: '0018. A separate encryption key'
parent: ADRs
nav_order: 18
---

# 0018. A separate encryption key

- Status: Accepted
- Date: 2026-10-20

## Context

Meerkat encrypts CardDAV and calendar passwords and TOTP secrets in the database, and hashes
recovery codes with an HMAC. All keys come from `JWT_SECRET_KEY` through SHA-256. One secret thus
has two jobs: a change of `JWT_SECRET_KEY` makes the stored secrets unreadable and locks out every
user with two-factor login. CodeQL also reports the SHA-256 key derivation
(`go/weak-sensitive-data-hashing`).

The key must not be in the database or the data directory. Backups contain the database and are
copied to other hosts, and a request that reads a file from the data directory could expose a key
there. Only the environment, like `JWT_SECRET_KEY`, keeps the key apart from the data.

## Decision

- A new environment variable `ENCRYPTION_KEY`: 32 random bytes, base64-encoded
  (`openssl rand -base64 32`). The server refuses to start if the value does not decode to 32 bytes,
  has fewer than 16 different byte values, or equals `JWT_SECRET_KEY`. Each error message gives the
  steps to fix it.
- HKDF-SHA256 derives two subkeys from it: one for AES-256-GCM, one for the recovery code HMAC.
- New ciphertexts have the prefix `v2:`. A value without the prefix uses the old key from
  `JWT_SECRET_KEY`.
- At startup with `ENCRYPTION_KEY`, Meerkat encrypts all old CardDAV and calendar passwords again
  with the new key, in one transaction. A value that does not decrypt stays unchanged and is counted
  in the log. Restored old backups are migrated the same way.
- A fixed text encrypted with the new key is stored in the database. If it does not decrypt at
  startup, the key is wrong and the server stops. If `v2:` values exist and `ENCRYPTION_KEY` is not
  set, the server stops too.
- `ENCRYPTION_KEY_RESET=true` with a new key is the way out when the key is lost: the server deletes
  the values of the lost key (two-factor data, `v2:` passwords, the fixed text) and starts with the
  new key. It does nothing when the key fits.
- TOTP secrets and recovery codes use only the new key. They are not migrated: no public deployment
  used two-factor login before this change.
- Without `ENCRYPTION_KEY` the server starts and logs an error with the steps to set it.
  `JWT_SECRET_KEY` does not change. A weak or placeholder `JWT_SECRET_KEY` gives a warning only.
- One year after the release, `ENCRYPTION_KEY` becomes required and the old key code is removed.

## Consequences

`JWT_SECRET_KEY` can change after the migration without loss of data. Admins must keep
`ENCRYPTION_KEY` with their `.env` and backups: without it the stored secrets are lost. After the
migration an older Meerkat version cannot read the passwords. Until the old key code is removed, the
CodeQL alert on it stays and is dismissed with a link to this ADR. CodeQL reports the recovery code
HMAC with the same rule. That alert is a false positive: the HMAC key is secret, the codes are
random and used once, and who has the key can already read the TOTP secrets. Backups older than one
year with passwords in the old format need the version before the removal to migrate them.
