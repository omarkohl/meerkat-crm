import { describe, test, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import MergeContactsDialog from './MergeContactsDialog';
import { planMerge, type MergeContact } from '../utils/mergeContacts';

vi.mock('../context/SnackbarContext', () => ({ useSnackbar: () => ({ showSuccess: vi.fn() }) }));

const contact = (ID: number, images: number): MergeContact =>
  ({
    ID,
    firstname: `Anna${String(ID)}`,
    images: Array.from({ length: images }, (_, i) => ({ id: ID * 100 + i, crop: null })),
  }) as unknown as MergeContact;

const contacts = [contact(1, 30), contact(2, 21)];

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

// @023-US3
describe('MergeContactsDialog', () => {
  test('refuses to start a merge that gives a contact more than 50 photos', () => {
    render(
      <DateFormatProvider>
        <MergeContactsDialog open onClose={vi.fn()} contactIds={[1, 2]} onMerged={vi.fn()} />
      </DateFormatProvider>,
    );
    expect(screen.getByRole('alert').textContent).toContain('51 photos');
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Continue' }).disabled).toBe(true);
  });
});
