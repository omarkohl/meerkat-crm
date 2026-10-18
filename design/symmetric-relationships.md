# Design: Symmetric and Inverse Relationships

Status: draft, for discussion.

Upstream: [fbuchner/meerkat-crm#185](https://github.com/fbuchner/meerkat-crm/issues/185) (deduce
relationship).

## Current state

- A `Relationship` is one directed row: owner `ContactID` → optional `RelatedContactID`.
- `Type` labels the _related_ person from the owner's view: on Jon, "Sister: Eva" means Eva is Jon's
  sister.
- The target contact shows the row as incoming: "Sister of Jon" (`GetIncomingRelationships`,
  `RelationshipList.tsx`).
- `Type` is free text. Presets (`RELATIONSHIP_TYPES` in `frontend/src/api/relationships.ts`) are
  stored in English and translated only at display time. Custom types are stored as typed
  ("Ehefrau").
- `Name`, `Gender`, `Birthday` are copied onto the row, also when it links a contact.
- Monica import already has to collapse reciprocal pairs (`isRedundantHalf`), because Monica stores
  both directions.
- CardDAV does not export relationships (no vCard `RELATED`).

## Problem

1. **Symmetric types read as one-sided.** Eva's page shows "Spouse of Jon" in a separate incoming
   list, not "Spouse: Jon". Users then add the reverse row by hand.
2. **Manual reverse rows duplicate.** With both rows, each page shows the spouse twice (outgoing +
   incoming) and the graph draws two edges.
3. **Inverse types are not inverted.** Jon → "Child: Eva" shows on Eva as "Child of Jon". Correct,
   but "Father: Jon" is what users expect. Gendered inverses (Son/Daughter, Mother/Father,
   Brother/Sister) depend on the gender of the _other_ person.
4. Unlinked rows (name only, no contact) have no other side. Symmetry does not apply to them.

The data is not wrong today. Problems 1 to 3 are about presentation and duplicate entry. Keep this
in mind when you compare the alternatives: a schema change fixes a display problem at a high cost.

## Building block: a type catalog

All alternatives except A0 need to know, per type, its inverse. Proposed catalog (keys = current
preset keys):

| Type                                                               | Inverse (from the other side)                         |
| ------------------------------------------------------------------ | ----------------------------------------------------- |
| Spouse, Partner, Sibling, Friend, Best Friend, Colleague, Neighbor | same (symmetric)                                      |
| Child                                                              | Parent; gendered: Father / Mother by owner's gender   |
| Parent, Mother, Father                                             | Child; gendered: Son / Daughter by owner's gender     |
| Brother, Sister                                                    | Sibling; gendered: Brother / Sister by owner's gender |
| Manager                                                            | Employee                                              |
| Employee                                                           | Manager                                               |
| Other, custom text                                                 | unknown → fall back to "X of Jon"                     |

Rule: inverse label uses the gender of the **owner contact** (Jon), because Eva sees Jon. Unknown
gender → neutral term (Parent, Child, Sibling).

Catalog variants:

- **C1 Hard-coded** in the backend (or shared constant). Simple, translatable. Custom types never
  get symmetry.
- **C2 Per-user editable**: each custom type can mark "symmetric" or name an inverse. Stored like
  `User.CustomFieldNames`. More power, more UI. Custom labels in other languages work.
- **C3 Per-row flag** `symmetric bool` chosen in the add dialog. Most flexible, but the user must
  decide every time, and two "Friend" rows can disagree.

Recommendation: C1 now. C2 only if users ask for custom inverses.

## Alternatives

### A0. Do nothing, document it

Explain in docs that one row is enough, and that the other contact shows it as incoming.

- \+ Zero cost.
- − Problems 1 to 3 stay. Users still add duplicates.

### A. Derived display (no schema change) — recommended

Keep one directed row. At read time, show incoming rows with the inverse label from the catalog.

- Contact page: merge outgoing and incoming into one list. Eva sees "Spouse: Jon" and "Father: Jon".
  Unknown inverse → "Colleague of Jon" as today.
- Add dialog: when the reverse row (or its inverse) already exists, block or warn ("Jon is already
  linked as Eva's spouse").
- Graph: draw one undirected edge for a symmetric pair. Merge A→B + B→A rows that are inverses of
  each other into one edge.
- Editing an incoming row edits the owner's row. The API routes are
  `/contacts/:ownerId/relationships/:id`, so the frontend must call them with the owner ID, which it
  already has (`contact_id`).
- Existing duplicate pairs: hide the second row at display time (same rule as the graph). Optional
  one-time cleanup action, no automatic delete.

Tradeoffs:

- \+ One source of truth. Delete, trash, restore, import, Monica stay unchanged.
- \+ Fixes existing data on read; no migration.
- − Only catalog types get the nice label.
- − The "owner" of a pair is still visible in some places (who can edit it, which contact's trash
  removes it).
- − Every reader (page, graph, future vCard export) must apply the same derivation. Put it in one
  backend function, not in the frontend.

### B. Mirror rows

On create of a linked catalog type, also create the reverse row. Link both with `inverse_id`.

- \+ Each contact owns its own row; the incoming list could disappear.
- \+ A future vCard `RELATED` export is per card and maps 1:1.
- − Two rows must stay in sync on edit, delete, trash, restore, permanent delete, Monica import, CSV
  import, user delete. Many code paths, high bug risk.
- − The copied `Name`/`Gender`/`Birthday` on the mirror describe the _owner_ contact, so they drift
  when that contact changes.
- − Migration must detect existing pairs and link them; unclear pairs need user input.
- − Contradicts the Monica importer's deliberate collapse.

### C. Edge model (undirected row with two labels)

Replace the row with `contact_a`, `contact_b`, `label_a` (what A is to B), `label_b` (what B is to
A). Symmetric = same label.

- \+ Cleanest model. Handles symmetric, inverse and odd pairs ("Mentor"/"Mentee") without a catalog.
- \+ One row, no sync problem.
- − Largest change: API, frontend, importers, export, trash, graph, tests, migration.
- − Unlinked rows (name only) do not fit; they stay a separate kind of row or get a nullable
  `contact_b`.
- − The user must enter two labels or the UI must fill one from a catalog anyway.

## Recommendation

**A + C1.** It fixes all three visible problems with no migration and no sync risk. If A later
proves too limited (for example custom inverse labels), move to C, not B. B has the worst
cost-to-benefit ratio.

## Open questions

1. Is per-type symmetry (catalog) enough, or do you want a per-relationship choice (C3)?
2. When the reverse already exists, must the add dialog **block** or only **warn**?
3. Must Eva's page show derived rows in the same list as her own rows, or keep a separate "incoming"
   section with better labels?
4. Should the graph show direction at all (arrows) for non-symmetric types like Manager?
5. Is a one-time "merge duplicate pairs" action wanted, or is hiding them at display time enough?
