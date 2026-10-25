import type { ContactResponse, ContactInput, ContactValue, ContactAddress } from '../api/contacts';
import { MAX_CONTACT_IMAGES } from '../api/images';

export type MergeContact = ContactResponse;

export const CUSTOM_FIELD_PREFIX = 'custom_fields.';
export const DECEASED_FIELD = 'deceased';
export const DECEASED_UNKNOWN_DATE = 'yes';

type ScalarKey =
  | 'firstname'
  | 'lastname'
  | 'nickname'
  | 'gender'
  | 'birthday'
  | 'anniversary'
  | 'prefix'
  | 'middle_name'
  | 'suffix'
  | 'organization'
  | 'department'
  | 'job_title'
  | 'role'
  | 'how_we_met'
  | 'food_preference'
  | 'work_information'
  | 'contact_information';

export const SCALAR_FIELDS: ScalarKey[] = [
  'firstname',
  'lastname',
  'nickname',
  'gender',
  'birthday',
  'anniversary',
  'prefix',
  'middle_name',
  'suffix',
  'organization',
  'department',
  'job_title',
  'role',
  'how_we_met',
  'food_preference',
  'work_information',
  'contact_information',
];

export interface ConflictValue {
  contactId: number;
  value: string;
}

export interface Conflict {
  field: string;
  values: ConflictValue[];
}

type NoteMove = MergeContact['notes'] extends (infer N)[] | null | undefined ? N : never;
type ReminderMove = MergeContact['reminders'] extends (infer R)[] | null | undefined ? R : never;
type RelationshipMove = MergeContact['relationships'] extends (infer R)[] | null ? R : never;

export interface MergeCounts {
  sources: number;
  emails: number;
  phones: number;
  addresses: number;
  urls: number;
  impps: number;
  circles: number;
  activities: number;
  notes: number;
  reminders: number;
  relationships: number;
}

export interface MergePlan {
  target: MergeContact;
  sources: MergeContact[];
  conflicts: Conflict[];
  defaults: Record<string, string>;
  fills: Record<string, string>;
  lists: {
    circles: string[];
    emails: ContactValue[];
    phones: ContactValue[];
    addresses: ContactAddress[];
    urls: ContactValue[];
    impps: ContactValue[];
    note: string;
    customFields: Record<string, string>;
  };
  moves: {
    activityIds: number[];
    notes: NoteMove[];
    reminders: ReminderMove[];
    relationships: RelationshipMove[];
    selfRelationships: RelationshipMove[];
  };
  counts: MergeCounts;
  imageCount: number;
  tooManyImages: boolean;
}

const trimmed = (s: string | null | undefined) => (s ?? '').trim();

function scalar(c: MergeContact, key: string): string {
  if (key === DECEASED_FIELD) return c.is_deceased ? c.deceased_date || DECEASED_UNKNOWN_DATE : '';
  if (key.startsWith(CUSTOM_FIELD_PREFIX)) return c.custom_fields?.[key.slice(CUSTOM_FIELD_PREFIX.length)] ?? '';
  return (c as unknown as Record<string, string | undefined>)[key] ?? '';
}

function union<T>(base: T[], more: T[], key: (item: T) => string): T[] {
  const seen = new Set(base.map(key));
  const out = [...base];
  for (const item of more) {
    const k = key(item);
    if (!seen.has(k)) {
      seen.add(k);
      out.push(item);
    }
  }
  return out;
}

const phoneKey = (v: string) => v.replace(/[^0-9+]/g, '');
const addressKey = (a: ContactAddress) =>
  [a.street, a.city, a.region, a.postal, a.country]
    .filter((p) => trimmed(p) !== '')
    .join(', ')
    .toLowerCase();

export function planMerge(target: MergeContact, sources: MergeContact[]): MergePlan {
  const all = [target, ...sources];
  const customKeys = [...new Set(all.flatMap((c) => Object.keys(c.custom_fields ?? {})))];
  const fields = [...SCALAR_FIELDS, DECEASED_FIELD, ...customKeys.map((k) => CUSTOM_FIELD_PREFIX + k)];

  const conflicts: Conflict[] = [];
  const defaults: Record<string, string> = {};
  const fills: Record<string, string> = {};
  for (const field of fields) {
    const values = all
      .filter((c) => trimmed(scalar(c, field)) !== '')
      .map((c) => ({ contactId: c.ID, value: scalar(c, field) }));
    const first = values[0];
    const distinct = new Set(values.map((v) => v.value.trim()));
    if (first && distinct.size > 1) {
      conflicts.push({ field, values });
      defaults[field] = first.value;
    } else if (first && trimmed(scalar(target, field)) === '') {
      fills[field] = first.value;
    }
  }

  const others = <T>(get: (c: MergeContact) => T[] | null | undefined) => sources.flatMap((c) => get(c) ?? []);
  const lower = (v: ContactValue) => v.value.trim().toLowerCase();
  const merge = (get: (c: MergeContact) => ContactValue[] | null | undefined, key: (v: ContactValue) => string) =>
    union(get(target) ?? [], others(get), key);

  const lists = {
    circles: union(
      target.circles ?? [],
      others((c) => c.circles),
      (c) => c,
    ),
    emails: merge((c) => c.emails, lower),
    phones: merge(
      (c) => c.phones,
      (v) => phoneKey(v.value),
    ),
    addresses: union(
      target.addresses ?? [],
      others((c) => c.addresses),
      addressKey,
    ),
    urls: merge((c) => c.urls, lower),
    impps: merge((c) => c.impps, lower),
    note: [...new Set(all.map((c) => trimmed(c.note)).filter((n) => n !== ''))].join('\n\n'),
    customFields: {} as Record<string, string>,
  };
  for (const c of all) {
    for (const [k, v] of Object.entries(c.custom_fields ?? {})) {
      if (!(k in lists.customFields) && trimmed(v) !== '') lists.customFields[k] = v;
    }
  }

  const ids = new Set(all.map((c) => c.ID));
  const haveActivity = new Set((target.activities ?? []).map((a) => a.ID));
  const noteKey = (n: { content: string; date: string }) => `${n.date}|${n.content}`;
  const haveNote = new Set((target.notes ?? []).map(noteKey));
  const reminderKey = (r: { message: string; remind_at: string; recurrence: string }) =>
    `${r.remind_at}|${r.recurrence}|${r.message}`;
  const haveReminder = new Set((target.reminders ?? []).map(reminderKey));
  const relKey = (r: { type: string; related_contact_id: number | null; name: string }) =>
    r.related_contact_id === null ? `${r.type}|${r.name.toLowerCase()}` : `${r.type}|#${String(r.related_contact_id)}`;
  const haveRel = new Set((target.relationships ?? []).map(relKey));

  const moves: MergePlan['moves'] = {
    activityIds: [],
    notes: [],
    reminders: [],
    relationships: [],
    selfRelationships: (target.relationships ?? []).filter(
      (r) => r.related_contact_id !== null && ids.has(r.related_contact_id),
    ),
  };
  for (const s of sources) {
    for (const a of s.activities ?? []) {
      if (!haveActivity.has(a.ID)) {
        haveActivity.add(a.ID);
        moves.activityIds.push(a.ID);
      }
    }
    for (const n of s.notes ?? []) {
      if (!haveNote.has(noteKey(n))) {
        haveNote.add(noteKey(n));
        moves.notes.push(n);
      }
    }
    for (const r of s.reminders ?? []) {
      if (!haveReminder.has(reminderKey(r))) {
        haveReminder.add(reminderKey(r));
        moves.reminders.push(r);
      }
    }
    for (const r of s.relationships ?? []) {
      if (r.related_contact_id !== null && ids.has(r.related_contact_id)) continue;
      if (!haveRel.has(relKey(r))) {
        haveRel.add(relKey(r));
        moves.relationships.push(r);
      }
    }
  }

  const added = (merged: unknown[], before: unknown[] | null | undefined) => merged.length - (before?.length ?? 0);
  // A photo that is not an image yet is uploaded as one during the merge.
  const imageCount = all.reduce((n, c) => n + (c.images?.length || (c.photo ? 1 : 0)), 0);
  return {
    target,
    sources,
    conflicts,
    defaults,
    fills,
    lists,
    moves,
    counts: {
      sources: sources.length,
      emails: added(lists.emails, target.emails),
      phones: added(lists.phones, target.phones),
      addresses: added(lists.addresses, target.addresses),
      urls: added(lists.urls, target.urls),
      impps: added(lists.impps, target.impps),
      circles: added(lists.circles, target.circles),
      activities: moves.activityIds.length,
      notes: moves.notes.length,
      reminders: moves.reminders.length,
      relationships: moves.relationships.length,
    },
    imageCount,
    tooManyImages: imageCount > MAX_CONTACT_IMAGES,
  };
}

// The full body of PUT /contacts/{id}: the update replaces every editable field.
export function buildMergedInput(plan: MergePlan, choices: Record<string, string>): ContactInput {
  const t = plan.target;
  const chosen = { ...plan.fills, ...plan.defaults, ...choices };
  const customFields = { ...plan.lists.customFields };
  const input: ContactInput & Record<string, unknown> = {
    email: t.email,
    phone: t.phone,
    address: t.address,
    circles: plan.lists.circles,
    emails: plan.lists.emails,
    phones: plan.lists.phones,
    addresses: plan.lists.addresses,
    urls: plan.lists.urls,
    impps: plan.lists.impps,
    note: plan.lists.note,
  };
  for (const key of SCALAR_FIELDS) input[key] = t[key];
  for (const [field, value] of Object.entries(chosen)) {
    if (field.startsWith(CUSTOM_FIELD_PREFIX)) customFields[field.slice(CUSTOM_FIELD_PREFIX.length)] = value;
    else if (field !== DECEASED_FIELD) input[field] = value;
  }
  const deceased = chosen[DECEASED_FIELD] ?? scalar(t, DECEASED_FIELD);
  input.is_deceased = deceased !== '';
  input.deceased_date = deceased === DECEASED_UNKNOWN_DATE ? '' : deceased;
  input['custom_fields'] = customFields;
  return input;
}

export function deceasedBeforeBirthday(plan: MergePlan, choices: Record<string, string>): boolean {
  const { birthday, deceased_date: deceased } = buildMergedInput(plan, choices);
  return !!deceased && /^\d{4}-\d{2}-\d{2}$/.test(birthday ?? '') && (birthday ?? '') > deceased;
}
