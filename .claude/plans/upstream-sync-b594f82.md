# Upstream sync: `0c2aaab8a979` → `b594f822924a`

Integrate upstream `main` up to `b594f822924a` by the process in
[fork.md](../../docs/development/fork.md#integrate-upstream) and
[ADR 0020](../../docs/adr/0020-upstream-integration.md). 44 non-merge upstream commits. One step is
one commit; tick it in this file in the same commit. Each commit passes `make precommit`.

## Read first

- `AGENTS.md`, `docs/development/fork.md`, ADR 0020, `upstream-ledger.yml`
- ADRs 0005 (user_id), 0006 (never edit a migration), 0009 (injected clock), 0011 (huma), 0012
  (modules)
- Port the **final upstream state** of a topic, not each commit: read the files with
  `git show b594f822924a:<path>` and the topic diff with
  `git diff 0c2aaab8a979 b594f822924a -- <paths>`. The upstream merge commits hold conflict fixes
  that only the final state shows.

## Settled

- Port these topics: deceased contacts, birthday reminder toggle, per-contact vCard export, vCard
  2.1 gate, README AI policy. The table in step 1 gives the status of all other commits.
- vCard export: port the final state, which has **no** version choice (`784262c`). Reason: the
  version-aware mapper changed the 3.0 output that CardDAV sync and hashing use. The fork keeps one
  mapper.
- Deceased date input: use the fork's `frontend/src/components/DateField.tsx`. Do **not** add
  `@mui/x-date-pickers` or `date-fns`.
- Upstream migrations 000024/000025 become fork migrations **000040/000041** with the same SQL. Do
  not copy the upstream files under their own numbers.
- A port commit has one `Upstream: <12-char hash>` footer for each upstream commit that it covers,
  and sets those ledger entries to `ported`.
- **Author**: a port commit keeps the upstream author. After `jj commit`, run
  `jj metaedit -r @- --author "Name <email>"`, and check it with `jj log -r @- -T 'author'`. When a
  port covers commits by several authors, the author is the person with most of the covered commits.
  Each other author gets a `Co-authored-by: Name <email>` trailer. The committer stays the fork
  maintainer. Authors: Ymage `<heltem@o2php.com>`, fredericbnr
  `<25030946+fbuchner@users.noreply.github.com>`.
- No issue URLs in changelog entries for ports: upstream issues are not fork issues.
- No conversion of upstream databases. Upstream down migrations can lose data. The fork only refuses
  such a database (step 2).
- A deceased contact keeps its birthday reminders and the toggle, as upstream. The user can turn the
  reminder off.

## Steps

- [x] **0. Environment.** `jj git fetch --remote upstream`; check `main@upstream` is (or contains)
      `b594f822924a`; `make precommit` is green on `omk`. If upstream moved on, stop and ask. Do not
      extend the range.

- [x] **1. Ledger and DIFFERENCES** (`chore: record upstream commits up to b594f82`). Add all 44
      entries to `upstream-ledger.yml` in upstream order
      (`jj log --reversed --no-graph -r '0c2aaab8a979..b594f822924a ~ merges()'`), with the status
      from the table below. Use `pending` for the commits that later steps port. In the same commit,
      update `DIFFERENCES.md` for what upstream took:
  - Overview rows for PR #247, #248, #249, #251 and #259: keep only what upstream does not have.
    Upstream dropped the Docker-free E2E stack (`e361be7`), the Prettier gate (`c2185e2`) and
    narrowed the linter set (`8bbfd25`). Compare `git show b594f822924a:.golangci.yml` and the
    workflows with the fork. Delete a row when nothing is left.
  - "CardDAV sync": upstream has unquoted ETags now. Keep only what the fork does in addition (weak
    ETags, error reporting), after you compare `backend/services/carddav*` with upstream.
  - #232: upstream fixed it (`89ade14`). Remove "fixes upstream bug #232" from Benefits, the
    Overview row and "Small UI fixes". The fork's date input stays a difference.
  - "Go 1.27. `go.mod` is raised from 1.25" → from 1.26.

  | Commit         | Status           | Reason                                                      |
  | -------------- | ---------------- | ----------------------------------------------------------- |
  | `39e18c3a747d` | pending (step 5) |                                                             |
  | `ff5354c0def6` | pending (step 4) |                                                             |
  | `c004af7306a9` | pending (step 4) |                                                             |
  | `daf2320f1c97` | pending (step 4) |                                                             |
  | `670a227dfcb2` | pending (step 4) |                                                             |
  | `598e8c744d8a` | pending (step 4) |                                                             |
  | `37aae9cab043` | pending (step 4) |                                                             |
  | `5f315a22f0ec` | pending (step 4) |                                                             |
  | `244a867077f7` | pending (step 4) |                                                             |
  | `c096156ac9e1` | skipped          | Fork uses its own DateField; no date-picker dependencies    |
  | `b38d404b95c8` | pending (step 4) |                                                             |
  | `648c932256f2` | pending (step 4) |                                                             |
  | `b30fa907bb6b` | pending (step 4) |                                                             |
  | `76b1d6bb87e2` | pending (step 4) |                                                             |
  | `2d6e91fa44c6` | pending (step 4) |                                                             |
  | `ed21b6da7066` | pending (step 4) |                                                             |
  | `259b1a9be9d1` | superseded       | Fork DateField uses the configured date format              |
  | `cdbf55b3fa4b` | pending (step 4) |                                                             |
  | `d1aa72155924` | pending (step 6) |                                                             |
  | `89ade1433093` | superseded       | Fork fixed #232 with DateField                              |
  | `a063a6deccae` | superseded       | Fork has migrate v4.20.1                                    |
  | `b99299aacf27` | superseded       | Fork has cors v1.7.9                                        |
  | `e64f5daafc6c` | superseded       | Fork PR #247                                                |
  | `64c4f30824d3` | superseded       | Fork PR #248                                                |
  | `27fc3c10a0b0` | superseded       | Fork PR #249                                                |
  | `4c371f191322` | superseded       | Fork PR #251                                                |
  | `ed8d68cac8dd` | superseded       | Fork uses Vite and npm; package not in the lockfile         |
  | `7c6a808a035b` | superseded       | Fork has source-map-js 1.2.2                                |
  | `b296e7b3e4f3` | skipped          | Fork has its own agent hooks; skill targets upstream layout |
  | `8a82df030d87` | skipped          | Upstream's own CI bot                                       |
  | `bf2fc29a7053` | pending (step 7) |                                                             |
  | `d64d1dc2a3db` | superseded       | Fork uses Vite and npm; package not in the lockfile         |
  | `914946071827` | superseded       | Fork has Vite 8                                             |
  | `3587e64178b4` | superseded       | Fork PR #259                                                |
  | `99af18849258` | superseded       | Fork has these versions or newer, and Go 1.27               |
  | `8bbfd251bef5` | skipped          | Fork keeps its stricter linter set                          |
  | `8955b46d7d81` | pending (step 3) |                                                             |
  | `784262cb8c21` | pending (step 6) |                                                             |
  | `e361be7e8505` | skipped          | Fork keeps the Docker-free E2E stack                        |
  | `7b951629cc0d` | superseded       | Fork pins golangci-lint v2.14.0 for Go 1.27                 |
  | `c2185e240845` | skipped          | Fork keeps the Prettier gate                                |
  | `fa7d09af438e` | superseded       | The files were never in the fork                            |
  | `da5e274032de` | superseded       | Fixes upstream's date-format change, which the fork skips   |
  | `d8b114a686ef` | pending (step 4) |                                                             |

  Check each "Fork has X" claim against `backend/go.mod`, `frontend/package-lock.json` and
  `.github/workflows` before you write it. If a claim is wrong, change the status to `pending` and
  write it in your report.

- [x] **2. Refuse upstream databases** (`feat: refuse to open a database migrated by upstream`).
      Fork-only, test first. In `backend/database/migrate.go`, add a check that runs in
      `RunMigrations` right after `m.Version()` and **before** the dirty `m.Force`: version ≥ 24 and
      no table `ics_feeds` (created by fork migration 000024) → return an error and write nothing.
      The error names the version, says that upstream Meerkat migrated the database, and links to
      the docs. Use the same check in `backend/cmd/migrate/main.go` before `up`/`down`.
  - Test: migrate a fresh database to 23 (`migrateTo`), apply upstream's 000024/000025 up SQL (copy
    to `backend/database/testdata/upstream/`), set `schema_migrations` to 25, not dirty. Then
    `RunMigrations` fails, and the version, the dirty flag, `sqlite_master` and the `contacts`
    columns stay the same. Also test the dirty case: upstream at 25, dirty. A fork database at 39
    and a fresh one must pass.
  - Check that opening the file does not change it (journal mode in `sqliteDSN`). If it changes, say
    so in the error docs, or move the check before the pragmas.
  - Docs: a short "Coming from upstream" section in `docs/development/fork.md` and the README
    installation part. `DIFFERENCES.md` Merge impact: fork migration numbers ≥ 24 differ from
    upstream's. CHANGELOG `Added`, one line.
  - Note: `release_dbs_test.go` will get an upstream v1.8.0 database later. The check must refuse
    it.

- [ ] **3. vCard 2.1 gate** (`fix: only rewrite vCard files that declare VERSION:2.1`, footer
      `8955b46d7d81`). Port `hasVCard21` and the gate in `backend/carddav/legacy.go`, with
      upstream's test `TestNormalizeLegacyVCF_LeavesFoldedModernCardAlone` first. Also check the
      fork's NOTE import with a folded 3.0 NOTE line (the fork imports NOTE, upstream does not).
      CHANGELOG `Fixed`.

- [ ] **4. Deceased contacts** (`feat: mark contacts as deceased`, footers: the 16 step-4 commits of
      the table). Final upstream state, adapted:
  - Migrations `000040_add_contact_deceased` and `000041_backfill_contact_deceased`: the up and down
    SQL of upstream 000024/000025, unchanged. Migration test like
    `backend/database/contact_note_migration_test.go`: the backfill takes `yes`, `YYYY-MM-DD` and
    `--MM-DD`, leaves `no` and `""`, and removes `Deceased` from `custom_field_names`; down restores
    it. The round-trip test must pass.
  - `models.Contact` and the contacts module (`domain/contact.go`, `adapters/http/input.go`,
    `dto.go`, `persistence.go` `showable`): `is_deceased`, `deceased_date`. Then `make openapi`.
  - Validation: format and "not before the birthday" as a `deceased_date` validator, next to
    `birthday` in `backend/middleware/validation.go`. "Not in the future" goes in the contacts use
    case with the injected `now` (ADR 0009), not `time.Now()`. Same error text as upstream.
  - Birthdays: `services.GetUpcomingBirthdays` excludes deceased contacts and relationships of
    deceased contacts (upstream `244a867`). That covers the dashboard and the birthday mail. The ICS
    feed (`services/ics_service.go` `BuildICSFeed`) also excludes them. Fork-only, test it.
  - Monica import: typed fields instead of the `Deceased` custom field
    (`services/monica_import_service.go`; check `monica_import_session.go` too).
  - Frontend: `contactFields.ts` key `is_deceased`; `ContactInformation.tsx` editor with a checkbox
    and `DateField` (`min` = birthday when it has a year; no future date); age at death
    (`calculateAgeAtDate` in `DateFormatProvider.tsx`, `now()` from `utils/now.ts`).
    `ContactDetailPage.tsx` `CONTACT_FIELDS` gets `is_deceased`, `deceased_date` (upstream
    `cdbf55b`). Check every `updateContact` caller (merge, bulk set, import) so that a save does not
    clear the flag.
  - `utils/mergeContacts.ts` / `MergeContactsDialog.tsx`: treat the two fields as one value, so a
    merge keeps them.
  - Strings: upstream's keys and texts in five locales (`contactDetail.deceased`,
    `contactDetail.deceasedDate`, `contactDetail.deceasedYes`, `contacts.deceased`).
  - Tests: upstream's Go tests (validator, controller, birthday service, Monica) moved to the module
    layout; upstream's `ContactInformation.test.tsx` cases; E2E in `contacts.spec.ts`: mark a
    contact deceased, edit another field, reload, still deceased; the contact leaves the dashboard
    birthdays. Use-case entry in `docs/use-cases.yml`.
  - CardDAV: `VCardToContact` changes the existing contact in place, so sync keeps the flag. Add a
    test for that. No vCard mapping (as upstream).
  - Docs: `docs/api-reference.md`; DIFFERENCES Merge impact: "upstream 000024/000025 are fork
    000040/000041". CHANGELOG `Added`.

- [ ] **5. Birthday reminder toggle** (`feat: toggle the birthday reminder on a contact`, footer
      `39e18c3a747d`). Port `frontend/src/utils/birthdayReminder.ts` with `now()` instead of
      `new Date()`, the switch in `ContactInformation.tsx` and the handlers in
      `ContactDetailPage.tsx`. `AddContactDialog.tsx` uses the shared helper. Change the birthday →
      move the reminder; clear the birthday → delete it. Show the switch for a deceased contact too
      (see Settled). Vitest for the helper (`findBirthdayReminder` picks the earliest match and
      ignores `reoccur_from_completion`). E2E in `reminders.spec.ts`: switch on → the reminder is in
      the Reminders tab; switch off → gone. Key `contactDetail.birthdayReminder` in five locales.
      CHANGELOG `Added`.

- [ ] **6. Per-contact vCard export** (`feat: export one contact as a vCard`, footers
      `d1aa72155924`, `784262cb8c21`).
  - Backend in the `exports` module: use case
    `ExportContactVCF(ctx, actor, contactID, now, client)`. Persistence loads the contact scoped by
    `user_id` and `deleted_at IS NULL` (trash), and encodes it with `carddav.ContactToVCard`, as the
    bulk export does. Not found → 404 `AppError`. Audit `EventExport` with detail
    `format:vcf contact:<id>`. Gin route `GET /contacts/:id/vcf` in `exports/internal/adapters/http`
    (file download, ADR 0012). Use the same param name as the contacts routes. Filename: upstream
    `contactFilenameSlug`.
  - `backend/main.go` CORS `ExposeHeaders` adds `Content-Disposition`.
  - `routes/invariants_test.go` entry and an isolation test: another user's contact and a trashed
    contact → 404. `docs/api-reference.md`.
  - Frontend: `exportContactAsVcf` in `api/export.ts`; menu item in `ContactHeader.tsx` as in
    upstream's final file, next to the fork's merge action. Take the strings from
    `git diff 0c2aaab8a979 b594f822924a -- frontend/src/i18n/locales/en.json`, in five locales.
    Check upstream's final `DataSettingsPage.tsx` diff. It should be empty after `784262c`.
  - E2E in `import-export.spec.ts`: open the contact by name, export, the download name is
    `<first>-<last>.vcf` and has the `FN`. CHANGELOG `Added`.

- [ ] **7. README AI policy** (`docs: align the AI policy with upstream`, footer `bf2fc29a7053`).
      Take upstream's paragraph into `README.md` (line ~131). `CONTRIBUTING.md` line 8 says the same
      in short form; make it agree.

- [ ] **8. Merge record.** Check that no ledger entry is `pending`. Then follow fork.md step 3 with
      `X = b594f822924a`, and in the same commit set `DIFFERENCES.md` "Last compared with upstream"
      to `b594f822924a`, with new numbers in the diff size line and the table (measure the same
      way). Check after the commit: `jj log -r 'b594f822924a..omk' ` lists only fork commits, and
      `git merge-base omk main` is `b594f822924a`.
