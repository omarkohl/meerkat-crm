import { ImportRowPreview } from '../api/import';

// Filters offered by the summary chips of the import preview.
export type PreviewFilter = 'add' | 'update' | 'skip' | 'errors' | 'warnings';

export type PreviewSortKey = 'row' | 'firstname' | 'lastname' | 'email' | 'status' | 'action';

export interface PreviewSort {
  key: PreviewSortKey;
  direction: 'asc' | 'desc';
}

// The status shown for a row, in the order a status sort lists them: problems first.
export type RowStatus = 'error' | 'deleted' | 'duplicate' | 'valid';
const STATUS_ORDER: RowStatus[] = ['error', 'deleted', 'duplicate', 'valid'];
const ACTION_ORDER = ['add', 'update', 'skip'];

export function rowStatus(row: ImportRowPreview): RowStatus {
  if (row.validation_errors.length > 0) return 'error';
  if (row.duplicate_match?.existing_deleted) return 'deleted';
  if (row.duplicate_match) return 'duplicate';
  return 'valid';
}

export function matchesFilter(row: ImportRowPreview, action: string, filter: PreviewFilter): boolean {
  switch (filter) {
    case 'errors':
      return row.validation_errors.length > 0;
    case 'warnings':
      return row.warnings.length > 0;
    default:
      return action === filter;
  }
}

// Row indices that match a filter at the moment it is selected. The view keeps
// this set, so a row does not vanish when its action is changed while filtered.
export function filterRowIndices(
  rows: ImportRowPreview[],
  actions: Map<number, string>,
  filter: PreviewFilter,
): Set<number> {
  return new Set(
    rows.filter((r) => matchesFilter(r, actions.get(r.row_index) || 'skip', filter)).map((r) => r.row_index),
  );
}

export function sortRows(
  rows: ImportRowPreview[],
  actions: Map<number, string>,
  sort: PreviewSort,
): ImportRowPreview[] {
  const value = (row: ImportRowPreview): string | number => {
    switch (sort.key) {
      case 'row':
        return row.row_index;
      case 'status':
        return STATUS_ORDER.indexOf(rowStatus(row));
      case 'action':
        return ACTION_ORDER.indexOf(actions.get(row.row_index) || 'skip');
      default:
        return (row.parsed_contact[sort.key] || '').toLowerCase();
    }
  };
  const sign = sort.direction === 'asc' ? 1 : -1;
  return [...rows].sort((a, b) => {
    const va = value(a);
    const vb = value(b);
    // Empty text sorts last in both directions, so blank cells do not hide the data.
    if (va === '' && vb !== '') return 1;
    if (vb === '' && va !== '') return -1;
    const cmp = typeof va === 'number' && typeof vb === 'number' ? va - vb : String(va).localeCompare(String(vb));
    return cmp !== 0 ? sign * cmp : a.row_index - b.row_index;
  });
}
