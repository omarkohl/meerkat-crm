import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import MergeContactsDialog from './MergeContactsDialog';
import { planMerge, type MergeContact } from '../utils/mergeContacts';

vi.mock('../context/SnackbarContext', () => ({ useSnackbar: () => ({ showSuccess: vi.fn() }) }));

let contacts = [
  { ID: 1, firstname: 'Anna', birthday: '1990-05-01', is_deceased: true, deceased_date: '1985-01-01' },
  { ID: 2, firstname: 'Anna', birthday: '1970-05-01', is_deceased: true, deceased_date: '1985-01-01' },
] as unknown as MergeContact[];
const conflicting = contacts;

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
function renderDialog() {
  return render(
    <DateFormatProvider>
      <MergeContactsDialog open onClose={vi.fn()} contactIds={[1, 2]} onMerged={vi.fn()} />
    </DateFormatProvider>,
  );
}

describe('MergeContactsDialog birthday after the deceased date', () => {
  beforeEach(() => {
    contacts = conflicting;
  });

  test('shows an error and blocks the merge until another birthday is picked', () => {
    renderDialog();
    const next = screen.getByRole<HTMLButtonElement>('button', { name: 'Continue' });
    expect(screen.getByRole('alert').textContent).toContain('after the date of death');
    expect(next.disabled).toBe(true);

    fireEvent.click(screen.getByRole('radio', { name: /^1970-05-01/ }));
    expect(screen.queryByRole('alert')).toBeNull();
    expect(next.disabled).toBe(false);
  });

  test('explains the block even when neither value is a conflict', () => {
    contacts = [
      { ID: 1, firstname: 'Anna', birthday: '1990-05-01' },
      { ID: 2, firstname: 'Anna', is_deceased: true, deceased_date: '1985-01-01' },
    ] as unknown as MergeContact[];
    renderDialog();
    expect(screen.getByRole('alert').textContent).toContain('after the date of death');
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Continue' }).disabled).toBe(true);
  });
});
