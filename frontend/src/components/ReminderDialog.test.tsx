import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import i18n from '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import ReminderDialog from './ReminderDialog';
import type { Reminder, ReminderFormData } from '../api/reminders';

const renderDialog = (props: Partial<React.ComponentProps<typeof ReminderDialog>> = {}) => {
  const onSave = vi.fn<(data: ReminderFormData) => Promise<void>>().mockResolvedValue(undefined);
  const onClose = vi.fn();
  render(
    <DateFormatProvider>
      <ReminderDialog open onClose={onClose} onSave={onSave} contactId={7} {...props} />
    </DateFormatProvider>,
  );
  return { onSave, onClose };
};

const type = (label: string, value: string) => {
  fireEvent.change(screen.getByRole('textbox', { name: new RegExp(label) }), { target: { value } });
};

const pickRecurrence = (name: string) => {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: /Recurrence|Repeat/i }));
  fireEvent.click(screen.getByRole('option', { name }));
};

const save = () => fireEvent.click(screen.getByRole('button', { name: 'Save' }));

describe('ReminderDialog', () => {
  beforeEach(async () => {
    window.localStorage.setItem('dateFormat', 'iso');
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-03-10T10:00:00Z'));
    await i18n.changeLanguage('en');
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  test('asks for a message before saving @006-US1', () => {
    const { onSave } = renderDialog();

    save();

    expect(screen.getByText('Message is required')).toBeTruthy();
    expect(onSave).not.toHaveBeenCalled();
  });

  test('saves a trimmed message for today by default and closes @006-US1', async () => {
    const { onSave, onClose } = renderDialog();
    type('Message', '  Call back  ');

    save();

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });
    expect(onSave).toHaveBeenCalledWith({
      message: 'Call back',
      by_mail: true,
      remind_at: new Date('2026-03-10').toISOString(),
      recurrence: 'once',
      reoccur_from_completion: true,
      contact_id: 7,
    });
  });

  test('a recurrence moves the date of a new reminder and offers rescheduling from completion @006-US1', async () => {
    const { onSave } = renderDialog();
    expect(screen.queryByRole('checkbox', { name: 'Reschedule from completion date' })).toBeNull();
    type('Message', 'Water the plants');

    pickRecurrence('Weekly');

    expect(screen.getByRole('checkbox', { name: 'Reschedule from completion date' })).toBeTruthy();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Send email notification' }));
    save();
    await waitFor(() => {
      expect(onSave).toHaveBeenCalled();
    });
    expect(onSave.mock.calls[0]?.[0]).toMatchObject({
      recurrence: 'weekly',
      by_mail: false,
      remind_at: new Date('2026-03-17').toISOString(),
    });
  });

  test('editing keeps the reminder date when the recurrence changes @006-US1', async () => {
    const reminder = {
      ID: 3,
      message: 'Pay rent',
      by_mail: false,
      remind_at: '2026-05-01T00:00:00Z',
      recurrence: 'once',
      reoccur_from_completion: false,
    } as Reminder;
    const { onSave } = renderDialog({ reminder });
    expect(screen.getByText('Edit Reminder')).toBeTruthy();
    expect(screen.getByDisplayValue('Pay rent')).toBeTruthy();

    pickRecurrence('Monthly');
    save();

    await waitFor(() => {
      expect(onSave).toHaveBeenCalled();
    });
    expect(onSave.mock.calls[0]?.[0]).toMatchObject({
      recurrence: 'monthly',
      by_mail: false,
      remind_at: new Date('2026-05-01').toISOString(),
    });
  });

  test('shows why saving failed and stays open @006-US1', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('Server said no'));
    const { onClose } = renderDialog({ onSave });
    type('Message', 'x');

    save();

    expect(await screen.findByText('Server said no')).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Save' })).toBeTruthy();
  });
});
