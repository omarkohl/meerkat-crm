import { describe, test, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import MergeContactsDialog from './MergeContactsDialog';
import { planMerge, type MergeContact } from '../utils/mergeContacts';

vi.mock('../context/SnackbarContext', () => ({ useSnackbar: () => ({ showSuccess: vi.fn() }) }));

const contacts = [
  { ID: 1, firstname: 'Anna', is_deceased: true },
  { ID: 2, firstname: 'Anna', is_deceased: true, deceased_date: '2020-01-15' },
] as unknown as MergeContact[];

vi.mock('../hooks/useContactMerge', () => ({
  useContactMerge: () => ({
    contacts,
    loading: false,
    error: null,
    merging: false,
    merge: vi.fn(),
    planFor: (id: number) =>
      planMerge(
        contacts.find((c) => c.ID === id) as MergeContact,
        contacts.filter((c) => c.ID !== id),
      ),
  }),
}));

// @004-US11
describe('MergeContactsDialog deceased conflict', () => {
  test('shows readable choices and no free-text option', () => {
    render(
      <DateFormatProvider>
        <MergeContactsDialog open onClose={vi.fn()} contactIds={[1, 2]} onMerged={vi.fn()} />
      </DateFormatProvider>,
    );
    expect(screen.getByRole('radio', { name: /^Yes/ })).toBeTruthy();
    expect(screen.getByRole('radio', { name: /15/ })).toBeTruthy();
    expect(screen.queryAllByRole('radio', { name: /Other value/ })).toHaveLength(0);
  });
});
