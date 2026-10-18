import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import i18n from '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import ReminderList from './ReminderList';
import type { Reminder } from '../api/reminders';

const reminder = (ID: number, extra: Partial<Reminder> = {}): Reminder =>
  ({
    ID,
    message: `Reminder ${String(ID)}`,
    by_mail: false,
    remind_at: '2031-05-01T00:00:00Z',
    recurrence: 'once',
    reoccur_from_completion: false,
    ...extra,
  }) as Reminder;

const renderList = (reminders: Reminder[]) => {
  const handlers = {
    onComplete: vi.fn().mockResolvedValue(undefined),
    onEdit: vi.fn(),
    onDelete: vi.fn().mockResolvedValue(undefined),
  };
  render(
    <DateFormatProvider>
      <ReminderList reminders={reminders} {...handlers} />
    </DateFormatProvider>,
  );
  return handlers;
};

describe('ReminderList', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
  });

  test('says so when there are no reminders @006-US1', () => {
    renderList([]);
    expect(screen.getByText('No reminders yet')).toBeTruthy();
  });

  test('shows how a reminder repeats and notifies @006-US1', () => {
    renderList([
      reminder(1),
      reminder(2, { recurrence: 'weekly', by_mail: true, reoccur_from_completion: true }),
      reminder(3, { recurrence: 'monthly', reoccur_from_completion: false }),
    ]);

    expect(screen.getAllByText('Weekly')).toHaveLength(1);
    expect(screen.getAllByText('Monthly')).toHaveLength(1);
    expect(screen.getAllByText('Email')).toHaveLength(1);
    expect(screen.getAllByText('Flexible')).toHaveLength(1);
  });

  test('completes and edits the reminder of the clicked card @006-US1', async () => {
    const { onComplete, onEdit } = renderList([reminder(1), reminder(2)]);

    fireEvent.click(screen.getAllByRole('button', { name: 'Complete' })[1] as HTMLElement);
    fireEvent.click(screen.getAllByRole('button', { name: 'Edit' })[0] as HTMLElement);

    await waitFor(() => {
      expect(onComplete).toHaveBeenCalledWith(2);
    });
    expect(onEdit).toHaveBeenCalledWith(expect.objectContaining({ ID: 1 }));
  });

  test('deletes only after the user confirms @006-US1', async () => {
    const { onDelete } = renderList([reminder(1)]);

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    expect(screen.getByText('Are you sure you want to delete this reminder?')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onDelete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
    await waitFor(() => {
      expect(onDelete).toHaveBeenCalledWith(1);
    });
  });
});
