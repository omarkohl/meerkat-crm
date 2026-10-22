import { describe, expect, test } from 'vitest';
import { dayOf, endDateToSend, endsBeforeStart } from './activityDates';

describe('endDateToSend', () => {
  test('sends an end date after the start', () => {
    expect(endDateToSend('2026-03-12', '2026-03-15')).toBe('2026-03-15T00:00:00.000Z');
  });

  test('sends null, which clears it, for no end, the start day or an earlier day', () => {
    expect(endDateToSend('2026-03-12')).toBeNull();
    expect(endDateToSend('2026-03-12', '')).toBeNull();
    expect(endDateToSend('2026-03-12', '2026-03-12')).toBeNull();
    expect(endDateToSend('2026-03-12', '2026-03-10')).toBeNull();
  });
});

describe('endsBeforeStart', () => {
  test('is true only for an end on an earlier day', () => {
    expect(endsBeforeStart('2026-03-12', '2026-03-10')).toBe(true);
    expect(endsBeforeStart('2026-03-12', '2026-03-12')).toBe(false);
    expect(endsBeforeStart('2026-03-12', '')).toBe(false);
    expect(endsBeforeStart('', '2026-03-10')).toBe(false);
  });
});

describe('dayOf', () => {
  test('cuts an API timestamp to its day', () => {
    expect(dayOf('2026-03-12T10:30:00Z')).toBe('2026-03-12');
    expect(dayOf(null)).toBe('');
    expect(dayOf(undefined)).toBe('');
  });
});
