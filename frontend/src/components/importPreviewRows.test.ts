import { describe, expect, it } from 'vitest';
import { DuplicateMatch, ImportRowPreview } from '../api/import';
import { filterRowIndices, sortRows } from './importPreviewRows';

const row = (row_index: number, firstname: string, extra: Partial<ImportRowPreview> = {}): ImportRowPreview => ({
  row_index,
  parsed_contact: firstname ? { firstname } : {},
  validation_errors: [],
  warnings: [],
  duplicate_match: null,
  suggested_action: 'add',
  ...extra,
});

const duplicate: DuplicateMatch = {
  existing_contact_id: 9,
  existing_firstname: 'Ada',
  existing_lastname: '',
  existing_email: '',
  existing_phone: '',
  match_reason: 'email',
  existing_deleted: false,
};

const rows = [
  row(0, 'carol'),
  row(1, 'Ada', { duplicate_match: duplicate }),
  row(2, '', { validation_errors: ['First name is required'] }),
  row(3, 'bob', { warnings: ['Invalid phone "*100#" not imported'] }),
];
const actions = new Map([
  [0, 'add'],
  [1, 'update'],
  [2, 'skip'],
  [3, 'add'],
]);

describe('filterRowIndices', () => {
  it('selects rows by their current action', () => {
    expect(filterRowIndices(rows, actions, 'add')).toEqual(new Set([0, 3]));
    expect(filterRowIndices(rows, actions, 'skip')).toEqual(new Set([2]));
  });

  it('selects rows with errors or warnings', () => {
    expect(filterRowIndices(rows, actions, 'errors')).toEqual(new Set([2]));
    expect(filterRowIndices(rows, actions, 'warnings')).toEqual(new Set([3]));
  });
});

describe('sortRows', () => {
  const order = (r: ImportRowPreview[]) => r.map((x) => x.row_index);

  it('sorts names ignoring case and keeps empty names last', () => {
    expect(order(sortRows(rows, actions, { key: 'firstname', direction: 'asc' }))).toEqual([1, 3, 0, 2]);
    expect(order(sortRows(rows, actions, { key: 'firstname', direction: 'desc' }))).toEqual([0, 3, 1, 2]);
  });

  it('lists problems first when sorting by status', () => {
    expect(order(sortRows(rows, actions, { key: 'status', direction: 'asc' }))).toEqual([2, 1, 0, 3]);
  });

  it('sorts by action, then by row', () => {
    expect(order(sortRows(rows, actions, { key: 'action', direction: 'asc' }))).toEqual([0, 3, 1, 2]);
  });
});
