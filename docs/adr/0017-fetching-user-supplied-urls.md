---
title: '0017. Fetching user-supplied image URLs'
parent: ADRs
nav_order: 17
---

# 0017. Fetching user-supplied image URLs

- Status: Accepted
- Date: 2026-10-20

## Context

The server fetches images from URLs that users choose: the image proxy of the profile picture dialog
(the browser cannot fetch them itself because of CORS), photo URLs in vCards (import, CardDAV
clients, contact sync) and gravatar URLs from a Monica import. Any user can supply such a URL, so
the server can be told to request any address it can reach (server-side request forgery). An
allow-list of hosts is not possible: users paste URLs from any site.

CodeQL reports `go/request-forgery` for this flow. It does not see a check made when the connection
is opened, so the alert stays and is dismissed as a false positive with a link to this ADR.

## Decision

`httputil.FetchImageFromURL` is the only way to fetch these images:

- Only `http` and `https` on ports 80 and 443, also for each redirect (at most two).
- `netguard` checks each address when it is dialled, also for redirects, and never uses a proxy.
  Internal addresses are refused whatever `BLOCK_PRIVATE_URLS` says. A check before the request
  would be weaker: a second DNS answer can differ from the first.
- At most four fetches run at a time on the server; others wait. Each has 15 s and 10 MB.
- The proxy allows 30 requests per minute per user (burst 5) and returns the image decoded and
  encoded again as JPEG, never the remote bytes. vCard photos are decoded and encoded again before
  they are stored.

## Consequences

Internal hosts cannot be reached. What remains: a logged-in user can make the server request public
hosts on ports 80 and 443, slowly, and see whether the request gave an image. A large vCard import
with photo URLs takes longer, because its fetches wait for a free slot. A private CardDAV server's
photo URLs do not load.
