# Design: "Myself" in Relationships and Activities

Status: draft, for discussion.

Upstream: [fbuchner/meerkat-crm#184](https://github.com/fbuchner/meerkat-crm/issues/184) (link the
current user with a contact).

## Current state

- A `User` has no linked contact. Relationships and activities always connect contacts with
  contacts.
- Many contact fields are already relative to the user: `HowWeMet`, Circles (Family, Work, …),
  reminders, notes. The user is the implicit center of the whole app.
- Activities link 1..n contacts. The user is never a participant. An activity with ≥2 contacts shows
  in the network graph.
- No feature uses activities to measure contact frequency ("last contacted", "stay in touch"). This
  matters for the activity part below.

## Why "myself"? Use cases

Before the model, decide which of these matter. They pull in different directions.

| #   | Use case                                                                      | Needs                                        |
| --- | ----------------------------------------------------------------------------- | -------------------------------------------- |
| U1  | Label kin and roles: "Eva is my sister", "Jon is my manager"                  | a typed label per contact                    |
| U2  | Network graph centered on me, showing how others connect through me           | a "me" node and edges to it                  |
| U3  | Activities record that _I_ was there (vs. events I only heard about)          | per-activity participation                   |
| U4  | Store my own data (birthday, addresses, photo) and share my card (vCard / QR) | a full contact record for me                 |
| U5  | Second-degree context: "Eva is the wife of my colleague Jon"                  | relationships between contacts (exists) + U1 |

Critical point: in a personal CRM _every_ contact relates to me. A generic "Friend of me" adds
little over Circles or `HowWeMet`. The value is in **specific** labels (U1) and in **visual
centering** (U2). U4 is useful but is a separate feature (profile / "my card").

## Part 1: How to model "myself"

### M0. No "myself"; document the workaround

Create a normal contact for yourself (as today).

- \+ Zero cost. Works for U1, U2, U4.
- − The self contact shows in lists, search, birthdays, dashboard, reminders, CardDAV sync.
  Archiving hides it, but archived contacts are not in the graph, so U2 breaks.
- − Nothing knows it is you, so no special behavior is possible.

### M1. Self contact: a normal contact, marked as me — recommended if U2 or U4 matter

Add `users.self_contact_id` (nullable FK). Alternative storage: `contacts.is_self` with a unique
index per user. The FK on the user is better: uniqueness comes free and a lookup does not scan
contacts.

Special treatment (each item is a decision, see open questions):

| Area                                       | Proposed behavior                                                                                                                                                |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Contact list, search                       | Hide by default, or pin on top with a "Me" badge                                                                                                                 |
| Dashboard, reminders, "upcoming birthdays" | Exclude from birthdays; reminders allowed (self-reminders are useful)                                                                                            |
| Graph                                      | Optional "center on me" mode; me node bigger. Off by default to avoid a star of edges                                                                            |
| Relationships                              | Normal. "Sister: Eva" on me = "Eva is my sister". With the symmetric design, Eva shows "Sibling: Me"                                                             |
| Delete / trash                             | Block, or clear `self_contact_id` first                                                                                                                          |
| Archive                                    | Block (archived = not in graph)                                                                                                                                  |
| CardDAV sync                               | Sync as normal; phones also have a "My Card". Risk: duplicate with an existing card for you; the "mark as me" action should let the user pick that existing card |
| Import (vCard/CSV/Monica)                  | Never sets "me" automatically. Monica may have its own "this is me" contact; check before mapping it                                                             |

- \+ Reuses everything: fields, photo, relationships, notes (a journal about yourself), vCard export
  (U4).
- \+ Small schema change; opt-in (no self contact → app works as today).
- − Special cases spread. Every list, count, birthday, reminder and sync query must decide "include
  me?". Missing one is a visible bug. Mitigate with one query scope (`excludeSelf`) and tests per
  feature.
- − Two-way FK `users` ↔ `contacts`; user delete must clear the FK first.

### M2. Virtual "me", not a contact

"Me" is not a row. Relationships get a target kind: `related_contact_id` OR
`related_to_self = true`. The graph adds a synthetic node from the user profile (username, optional
avatar).

- \+ No fake contact in lists, sync, counts. No special cases outside relationships and the graph.
- − No profile data for me (birthday, addresses) → U4 not covered.
- − Direction becomes awkward. With today's model, the row is owned by a contact, so "Jon → Son: me"
  reads "I am Jon's son". To write "Jon is my father" you need an owner "me", which means a nullable
  `ContactID`. Every relationship query gets a third branch.
- − The symmetric design must handle a node that has no contact page.

### M3. A "relationship to me" field on the contact

Add `Contact.relation_to_me` (catalog value or free text, maybe multi-value). Like `HowWeMet`, but
structured.

- \+ Very small. Matches the "everything is relative to me" nature of the app. Filterable and
  sortable in the contact list.
- \+ Graph can draw a synthetic "me" node with edges from this field (U2) without a self contact.
- − Overlaps with Circles. A contact who is colleague and friend needs multi-value; then it is
  almost Circles with labels.
- − No U4. Not reusable for activities.

### Comparison

|     | U1 labels | U2 graph        | U3 activities | U4 my data | Cost | Special cases            |
| --- | --------- | --------------- | ------------- | ---------- | ---- | ------------------------ |
| M0  | yes       | partly          | no            | yes        | none | none, but polluted lists |
| M1  | yes       | yes             | possible      | yes        | M    | many                     |
| M2  | yes       | yes             | possible      | no         | M–L  | few, but deep            |
| M3  | yes       | yes (synthetic) | no            | no         | S    | few                      |

Recommendation: **M3** if you mostly want U1/U2. **M1** if you also want U4 (your own card,
journal). Avoid M2: it costs about as much as M1 and gives less.

## Part 2: Do activities include me?

Question: if I did not take part, why record it?

Valid reasons to record an activity without me:

- **Life events of contacts**: "Jon and Eva married", "Eva moved to Berlin", "Jon's father died".
  This is often the most important context in a personal CRM.
- **Network knowledge**: "Jon and Eva met at FOSDEM". This is why the graph links contacts through
  activities.
- **Things I arranged but did not attend**: "Introduced Jon to Eva" (arguably I took part).

Most activities are still _my_ interactions (dinner, call, meeting). So "me" is the normal case and
"not me" the exception.

### A1. Me is always implicit (status quo, documented) — recommended now

- Activities mean "something happened with these contacts". Usually with me, sometimes not.
- Life events go to notes (dated), or later to a dedicated "life event" kind.
- \+ No change. No extra checkbox.
- − No way to tell interactions from hearsay. That only matters when a feature needs it (see A2).

### A2. Per-activity flag `with_me` (default true)

- \+ Separates interactions from observed events. Enables future "last interacted" / "stay in touch"
  features, which must count only real interactions.
- \+ Independent of the Part 1 choice (works with M0 to M3).
- − One more control on every activity, almost never changed.
- − No consumer exists today. Build it together with a "last contacted" feature, not before.

### A3. Me as a normal participant (needs M1)

Add the self contact to `activity_contacts`.

- − Every activity links to me: the self contact page lists all activities; the graph turns into a
  star, and one-contact activities suddenly appear in the graph (≥2 contacts).
- − So the graph, timeline and counts need special cases for me anyway. Then it is A2 with more
  complex data.
- \+ Only one advantage: uniform data for exports.

Do not choose A3.

### A4. Activity kinds: interaction vs. event

Replace the boolean with `kind`: `interaction` (with me), `event` (about contacts), maybe
`life_event` (one contact, shows in a separate timeline lane or on the birthday/anniversary
calendar).

- \+ Richer than A2. Life events (wedding, new job) could repeat yearly like birthdays ("Jon and
  Eva: 5th anniversary").
- − Bigger UI and import design. Overlaps with notes.

## Recommendation

1. Part 1: decide by use case (see open question 1). Default proposal: **M3** first; **M1** later if
   U4 becomes a goal.
2. Part 2: **A1** now, documented. Add **A2** (or **A4**) together with a "last contacted" feature.

## Open questions

1. Which use cases (U1 to U5) do you actually want? This decides M1 vs. M3.
2. M1: must the self contact be hidden from the contact list, or pinned with a "Me" badge?
3. M1: must the self contact sync to CardDAV? If yes, how do we avoid a duplicate of the card for
   you that already exists on your phone?
4. M3: single value or multi-value "relation to me"? Should it replace or extend Circles?
5. Do you want a "last contacted" / "stay in touch" feature? If no, A2 has no value.
6. Life events: notes, activities, or a new kind (A4) with yearly anniversaries?
