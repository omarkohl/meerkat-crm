import { describe, test, expect, vi, beforeEach } from 'vitest';
import { useState } from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import EditTimelineItemDialog, { TimelineItemValues } from './EditTimelineItemDialog';
import { getContacts } from '../api/contacts';

vi.mock('../api/contacts', () => ({ getContacts: vi.fn() }));

const contact = (ID: number, firstname: string, lastname: string) => ({ ID, firstname, lastname });

const Harness = () => {
  const [values, setValues] = useState<TimelineItemValues>({
    activityTitle: 'Lunch',
    activityContacts: [contact(1, 'Alice', 'Ames')],
  });
  return (
    <EditTimelineItemDialog
      open
      onClose={vi.fn()}
      onSave={vi.fn()}
      onDelete={vi.fn()}
      type="activity"
      values={values}
      onChange={setValues}
    />
  );
};

describe('EditTimelineItemDialog contact picker', () => {
  beforeEach(() => {
    vi.mocked(getContacts).mockReset();
    vi.mocked(getContacts).mockImplementation((({ search }: { search?: string }) =>
      Promise.resolve({
        contacts: search === 'Zed' ? [contact(2, 'Zed', 'Zimmer')] : [],
      })) as unknown as typeof getContacts);
  });

  test('offers a contact that the server finds for the typed term @005-US2', async () => {
    render(
      <DateFormatProvider>
        <Harness />
      </DateFormatProvider>,
    );

    const input = screen.getByRole('combobox', { name: /contacts/i });
    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: 'Zed' } });

    const option = await screen.findByRole('option', { name: 'Zed Zimmer' });
    fireEvent.click(option);

    expect(screen.getByRole('button', { name: 'Zed Zimmer' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Alice Ames' })).toBeTruthy();
  });
});
