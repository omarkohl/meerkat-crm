---
title: Frontend Development
parent: Development
nav_order: 3
---

# Frontend Development

## Setup

Requires the Node.js version in `.node-version` (it includes npm). fnm reads that file; mise reads
it after `mise settings add idiomatic_version_file_enable_tools node`. npm refuses to install on an
older Node (`engine-strict` in `frontend/.npmrc`).

```sh
cd frontend
cp .env.example .env  # set VITE_API_URL if backend isn't on localhost:8080
npm ci
```

Dependency install scripts are blocked unless `allowScripts` in `package.json` allows them (manage
it with `npm install-scripts`).

## Running Locally

```sh
npm start  # dev server on port 7300
```

Hot reload is enabled. The dev server proxies nothing so requests go directly to `VITE_API_URL`.

## Adding a New Page

1. Create `src/FooPage.tsx`.
2. Add a route in the router (typically in `App.tsx`).
3. Add a custom hook in `src/hooks/useFoo.ts` for data fetching (see pattern below).
4. Add an API module in `src/api/foo.ts` if needed.

## API Client

API types come from the OpenAPI document: derive them from `src/types/generated/api.ts`
(`components['schemas']['NoteResponse']`) instead of writing them by hand, and run `make openapi`
after the backend changes. All requests go through `src/api/client.ts`. It handles:

- httpOnly cookie auth (`credentials: 'include'`, no Authorization header)
- Configurable timeout via `VITE_REQUEST_TIMEOUT` (default 30s)
- Automatic redirect to `/login` on 401
- Structured `ApiError` with `code` and `details` fields

Call the client from API modules, not directly from components.

## Custom Hooks

Hooks in `src/hooks/` encapsulate data fetching. Hooks own their loading/error state. Components
call `refetch()` after mutations.

## Components

Reusable components live in `src/components/`. They are MUI-based and receive data via props and do
not fetch data themselves. Dialog components manage their own open/close state when passed an `open`
prop and `onClose` callback.

## Translations

All user-facing strings must be translated. Add keys to all five files in `src/i18n/locales/` (en,
de, it, es, fr); `make i18n-check` fails on a missing key. Do not hardcode English strings in
components.

## Linting

`make lint-frontend` runs ESLint 9 (`frontend/eslint.config.mjs`) on `src/**/*.{ts,tsx}`:
`typescript-eslint` strict-type-checked, `react-hooks`, `jsx-a11y`, `eslint-comments`. Every rule is
an error. `e2e/` is linted with `eslint-plugin-playwright` and two extra rules (no template-literal
`goto`, no `getByTestId`), see [Testing](testing.md).

### Suppressions (existing findings)

Findings that existed when the rules were introduced are recorded as ESLint bulk suppressions in
`frontend/eslint-suppressions.json`: a count per file and rule. A file with a suppression for a rule
may not get more violations of it (ESLint then reports all of that rule's violations in the file).
Fixing violations makes the suppression stale and `make lint-frontend` fails with "There are
suppressions left that do not occur anymore": run `cd frontend && npx eslint --prune-suppressions`
and commit the smaller file. Never run `--suppress-all` to silence new code. For one justified
exception use a line comment with a description: `// eslint-disable-next-line rule -- why`.

Why each rule class is suppressed today (fix the cause, not the count):

| Rule                                                                                                                                                                                                          | Reason                                                                                           |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `restrict-template-expressions`                                                                                                                                                                               | Numbers, unknown and nullable values interpolated into strings. Use `String()` or types.         |
| `no-misused-promises`                                                                                                                                                                                         | Async functions passed as event handlers expecting void. Wrap in a handler that catches.         |
| `no-floating-promises`                                                                                                                                                                                        | Promises started without await, catch or `void`. Await, or `void` plus a catch.                  |
| `no-unsafe-return`, `no-unsafe-assignment`, `no-unsafe-member-access`, `no-unsafe-argument`, `no-unsafe-call`                                                                                                 | `handleResponse` returns `Record<string, any>` and graph refs are `any`. Type the API responses. |
| `no-unnecessary-condition`                                                                                                                                                                                    | Defensive checks the types say cannot happen. Type the data as it really is, or delete.          |
| `no-deprecated`                                                                                                                                                                                               | Deprecated MUI props and React types. Move to the replacement API.                               |
| `no-non-null-assertion`                                                                                                                                                                                       | Assertions on values checked elsewhere. Use a guard or a narrowed variable.                      |
| `no-empty-object-type`, `no-explicit-any`, `prefer-promise-reject-errors`, `use-unknown-in-catch-callback-variable`, `no-unused-expressions`, `no-confusing-void-expression`, `no-unnecessary-type-assertion` | Single leftovers in a few files. Fix when touching the file.                                     |
| `jsx-a11y/no-autofocus`                                                                                                                                                                                       | Dialog fields use `autoFocus` to start typing at once. Decide per dialog.                        |

## Architecture rules

`make arch` runs `dependency-cruiser` on `src` (`frontend/.dependency-cruiser.cjs`): no circular
imports, no unresolvable imports, pages (`src/*Page.tsx`) call `src/api` only through hooks, and
nothing outside `src/api` imports `src/api/client.ts`. Existing violations are recorded per pair in
`frontend/.dependency-cruiser-known-violations.json`, with the reason per rule in
[ADR 0010](../adr/0010-target-layering.md). A new violation fails; when you fix a recorded one, run
`node scripts/arch-frontend.mjs --update` to drop it. Never add to the file.

## Testing

Vitest unit tests sit next to the code (`*.test.ts(x)`, `make test-frontend`); Playwright E2E tests
drive the running application. See [Testing](testing.md).
