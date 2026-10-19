import { describe, test, expect } from 'vitest';
import { planMerge, buildMergedInput, type MergeContact } from './mergeContacts';

function contact(ID: number, overrides: Partial<MergeContact> = {}): MergeContact {
  return {
    ID,
    firstname: 'Anna',
    lastname: '',
    circles: [],
    emails: [],
    phones: [],
    addresses: [],
    urls: [],
    impps: [],
    custom_fields: {},
    notes: [],
    activities: [],
    reminders: [],
    relationships: [],
    ...overrides,
  } as MergeContact;
}

describe('planMerge conflicts', () => {
  test('only fields with two different non-empty values are conflicts, target first', () => {
    const target = contact(1, { lastname: 'Smith', job_title: 'Dev', nickname: 'A' });
    const source = contact(2, { lastname: 'Jones', job_title: 'Dev', organization: 'ACME' });
    const plan = planMerge(target, [source]);
    expect(plan.conflicts).toEqual([
      {
        field: 'lastname',
        values: [
          { contactId: 1, value: 'Smith' },
          { contactId: 2, value: 'Jones' },
        ],
      },
    ]);
    expect(plan.defaults).toEqual({ lastname: 'Smith' });
  });

  test('a field the target leaves empty is filled from a source, not a conflict', () => {
    const plan = planMerge(contact(1), [contact(2, { job_title: 'Dev' })]);
    expect(plan.conflicts).toEqual([]);
    expect(buildMergedInput(plan, {}).job_title).toBe('Dev');
  });

  test('custom fields conflict by key', () => {
    const target = contact(1, { custom_fields: { pet: 'cat', car: 'vw' } });
    const source = contact(2, { custom_fields: { pet: 'dog', car: 'vw', boat: 'yes' } });
    const plan = planMerge(target, [source]);
    expect(plan.conflicts.map((c) => c.field)).toEqual(['custom_fields.pet']);
    const input = buildMergedInput(plan, { 'custom_fields.pet': 'dog' });
    expect(input.custom_fields).toEqual({ pet: 'dog', car: 'vw', boat: 'yes' });
  });

  test('the chosen or typed value wins', () => {
    const plan = planMerge(contact(1, { lastname: 'Smith' }), [contact(2, { lastname: 'Jones' })]);
    expect(buildMergedInput(plan, { lastname: 'Jones-Smith' }).lastname).toBe('Jones-Smith');
    expect(buildMergedInput(plan, {}).lastname).toBe('Smith');
  });
});

describe('planMerge lists', () => {
  test('values are united once, target first', () => {
    const target = contact(1, {
      circles: ['Friends'],
      emails: [{ type: 'home', value: 'a@example.com' }],
      phones: [{ type: '', value: '+49 30 1234' }],
      addresses: [{ type: '', street: 'Main St', city: 'Berlin', region: '', postal: '', country: '' }],
      note: 'likes tea',
    });
    const source = contact(2, {
      circles: ['Friends', 'Work'],
      emails: [
        { type: '', value: 'A@example.com' },
        { type: '', value: 'b@example.com' },
      ],
      phones: [{ type: '', value: '+4930 1234' }],
      addresses: [{ type: '', street: 'main st', city: 'berlin', region: '', postal: '', country: '' }],
      urls: [{ type: '', value: 'https://example.com' }],
      note: 'plays chess',
    });
    const plan = planMerge(target, [source]);
    const input = buildMergedInput(plan, {});
    expect(input.circles).toEqual(['Friends', 'Work']);
    expect(input.emails).toEqual([
      { type: 'home', value: 'a@example.com' },
      { type: '', value: 'b@example.com' },
    ]);
    expect(input.phones).toHaveLength(1);
    expect(input.addresses).toHaveLength(1);
    expect(input.urls).toEqual([{ type: '', value: 'https://example.com' }]);
    expect(input.note).toBe('likes tea\n\nplays chess');
    expect(plan.counts).toMatchObject({ emails: 1, phones: 0, addresses: 0, urls: 1, circles: 1, sources: 1 });
  });

  test('identical notes are joined once', () => {
    const plan = planMerge(contact(1, { note: 'same' }), [contact(2, { note: 'same' }), contact(3, { note: 'other' })]);
    expect(buildMergedInput(plan, {}).note).toBe('same\n\nother');
  });
});

describe('planMerge moves', () => {
  const day = '2025-01-02T00:00:00Z';
  test('activities, notes and reminders move unless the target has them already', () => {
    const act = (ID: number) => ({ ID, title: 't', description: '', location: '', date: day }) as never;
    const note = (ID: number, content: string, contact_id: number) => ({ ID, content, date: day, contact_id }) as never;
    const rem = (ID: number, message: string, contact_id: number) =>
      ({ ID, message, remind_at: day, recurrence: 'once', contact_id }) as never;
    const target = contact(1, {
      activities: [act(10)],
      notes: [note(20, 'same', 1)],
      reminders: [rem(30, 'call', 1)],
    });
    const source = contact(2, {
      activities: [act(10), act(11)],
      notes: [note(21, 'same', 2), note(22, 'new', 2)],
      reminders: [rem(31, 'call', 2), rem(32, 'write', 2)],
    });
    const plan = planMerge(target, [source]);
    expect(plan.moves.activityIds).toEqual([11]);
    expect(plan.moves.notes.map((n) => n.ID)).toEqual([22]);
    expect(plan.moves.reminders.map((r) => r.ID)).toEqual([32]);
    expect(plan.counts).toMatchObject({ activities: 1, notes: 1, reminders: 1 });
  });

  test('relationships to the merged contacts are dropped, duplicates too', () => {
    const rel = (ID: number, contact_id: number, related: number | null, type: string, name = 'X') =>
      ({ ID, contact_id, related_contact_id: related, type, name, gender: '', birthday: '' }) as never;
    const target = contact(1, {
      relationships: [rel(1, 1, 5, 'friend'), rel(2, 1, 2, 'colleague')],
    });
    const source = contact(2, {
      relationships: [
        rel(3, 2, 5, 'friend'),
        rel(4, 2, 6, 'sibling'),
        rel(5, 2, 1, 'colleague'),
        rel(6, 2, null, 'cousin', 'Zed'),
      ],
    });
    const plan = planMerge(target, [source]);
    expect(plan.moves.relationships.map((r) => r.ID)).toEqual([4, 6]);
    expect(plan.moves.selfRelationships.map((r) => r.ID)).toEqual([2]);
    expect(plan.counts.relationships).toBe(2);
  });
});
