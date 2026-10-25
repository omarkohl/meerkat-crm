import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Reminder } from '../api/reminders';
import {
  buildBirthdayReminderPayload,
  computeNextBirthdayOccurrence,
  findBirthdayReminder,
  parseBirthdayMonthDay,
} from './birthdayReminder';

const reminder = (ID: number, remind_at: string, overrides: Partial<Reminder> = {}): Reminder =>
  ({
    ID,
    message: 'Birthday',
    recurrence: 'yearly',
    reoccur_from_completion: false,
    remind_at,
    ...overrides,
  }) as Reminder;

// @006-US1
describe('birthdayReminder', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2031, 5, 10, 12, 0, 0));
  });
  afterEach(() => vi.useRealTimers());

  it('reads month and day from full and year-less birthdays', () => {
    expect(parseBirthdayMonthDay('1990-03-04')).toEqual({ month: 2, day: 4 });
    expect(parseBirthdayMonthDay('--03-04')).toEqual({ month: 2, day: 4 });
    expect(parseBirthdayMonthDay('')).toBeUndefined();
    expect(parseBirthdayMonthDay('nonsense')).toBeUndefined();
  });

  it('puts the next occurrence at 09:00, this year or next', () => {
    const later = computeNextBirthdayOccurrence('1990-12-24');
    expect([later?.getFullYear(), later?.getMonth(), later?.getDate(), later?.getHours()]).toEqual([2031, 11, 24, 9]);
    const earlier = computeNextBirthdayOccurrence('--01-02');
    expect([earlier?.getFullYear(), earlier?.getMonth(), earlier?.getDate()]).toEqual([2032, 0, 2]);
  });

  it('builds a yearly mail reminder for the contact', () => {
    const payload = buildBirthdayReminderPayload(7, '--12-24', 'Birthday of Anna');
    expect(payload).toMatchObject({
      message: 'Birthday of Anna',
      by_mail: true,
      recurrence: 'yearly',
      reoccur_from_completion: false,
      contact_id: 7,
    });
    expect(buildBirthdayReminderPayload(7, 'nonsense', 'x')).toBeUndefined();
  });

  it('finds the earliest yearly reminder on the birthday and ignores the others', () => {
    const reminders = [
      reminder(1, new Date(2032, 11, 24, 9).toISOString()),
      reminder(2, new Date(2031, 11, 24, 9).toISOString()),
      reminder(3, new Date(2030, 11, 24, 9).toISOString(), { reoccur_from_completion: true }),
      reminder(4, new Date(2030, 11, 24, 9).toISOString(), { recurrence: 'once' }),
      reminder(5, new Date(2030, 10, 24, 9).toISOString()),
    ];
    expect(findBirthdayReminder(reminders, '1990-12-24')?.ID).toBe(2);
    expect(findBirthdayReminder(reminders, '1990-01-01')).toBeUndefined();
  });
});
