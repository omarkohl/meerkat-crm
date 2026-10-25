import { describe, test, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  calculateAgeFromBirthday,
  calculateAgeAtDate,
  formatDateWithFormat,
  formatDateRangeWithFormat,
  formatBirthdayWithFormat,
  formatBirthdayForInputWithFormat,
  parseBirthdayInputWithFormat,
  autoFormatBirthdayInputWithFormat,
} from './DateFormatProvider';

describe('formatDateWithFormat', () => {
  test('formats ISO dates in EU and US styles', () => {
    expect(formatDateWithFormat('2026-06-11', 'eu')).toBe('11.06.2026');
    expect(formatDateWithFormat('2026-06-11', 'us')).toBe('06/11/2026');
    expect(formatDateWithFormat('2026-06-11', 'iso')).toBe('2026-06-11');
  });

  test('returns empty string for empty input', () => {
    expect(formatDateWithFormat('', 'eu')).toBe('');
  });

  test('returns unparseable input unchanged', () => {
    expect(formatDateWithFormat('not-a-date', 'eu')).toBe('not-a-date');
  });
});

describe('formatDateRangeWithFormat', () => {
  test('shows a single date without an end or when the end is the same day', () => {
    expect(formatDateRangeWithFormat('2026-03-12', undefined, 'eu')).toBe('12.03.2026');
    expect(formatDateRangeWithFormat('2026-03-12', null, 'eu')).toBe('12.03.2026');
    expect(formatDateRangeWithFormat('2026-03-12T10:00:00Z', '2026-03-12T00:00:00Z', 'us')).toBe('03/12/2026');
  });

  test('leaves out what start and end share in EU and US styles', () => {
    expect(formatDateRangeWithFormat('2026-03-12', '2026-03-15', 'eu')).toBe('12.–15.03.2026');
    expect(formatDateRangeWithFormat('2026-02-28', '2026-03-03', 'eu')).toBe('28.02.–03.03.2026');
    expect(formatDateRangeWithFormat('2025-12-28', '2026-01-03', 'eu')).toBe('28.12.2025 – 03.01.2026');
    expect(formatDateRangeWithFormat('2026-03-12', '2026-03-15', 'us')).toBe('03/12–15/2026');
    expect(formatDateRangeWithFormat('2026-02-28', '2026-03-03', 'us')).toBe('02/28–03/03/2026');
    expect(formatDateRangeWithFormat('2025-12-28', '2026-01-03', 'us')).toBe('12/28/2025 – 01/03/2026');
  });

  test('keeps both full dates in the ISO style', () => {
    expect(formatDateRangeWithFormat('2026-03-12', '2026-03-15', 'iso')).toBe('2026-03-12 – 2026-03-15');
  });

  test('returns unparseable input unchanged', () => {
    expect(formatDateRangeWithFormat('not-a-date', '2026-03-15', 'eu')).toBe('not-a-date');
  });
});

describe('formatBirthdayWithFormat', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 5, 11)); // 2026-06-11
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('formats full birthdays in EU and US styles', () => {
    expect(formatBirthdayWithFormat('1990-04-30', 'eu')).toBe('30.04.1990');
    expect(formatBirthdayWithFormat('1990-04-30', 'us')).toBe('04/30/1990');
    expect(formatBirthdayWithFormat('1990-04-30', 'iso')).toBe('1990-04-30');
  });

  test('formats year-less birthdays', () => {
    expect(formatBirthdayWithFormat('--04-30', 'eu')).toBe('30.04.');
    expect(formatBirthdayWithFormat('--04-30', 'us')).toBe('04/30');
    expect(formatBirthdayWithFormat('--04-30', 'iso')).toBe('04-30');
  });

  test('appends the age when requested', () => {
    expect(formatBirthdayWithFormat('1990-04-30', 'eu', true)).toBe('30.04.1990 (36)');
    expect(formatBirthdayWithFormat('1990-04-30', 'us', true)).toBe('04/30/1990 (36)');
    expect(formatBirthdayWithFormat('1990-04-30', 'iso', true)).toBe('1990-04-30 (36)');
  });

  test('age accounts for a birthday later in the year', () => {
    expect(formatBirthdayWithFormat('1990-12-31', 'eu', true)).toBe('31.12.1990 (35)');
    expect(formatBirthdayWithFormat('1990-12-31', 'us', true)).toBe('12/31/1990 (35)');
    expect(formatBirthdayWithFormat('1990-12-31', 'iso', true)).toBe('1990-12-31 (35)');
  });

  test('returns unrecognized values unchanged', () => {
    expect(formatBirthdayWithFormat('whenever', 'eu')).toBe('whenever');
  });
});

describe('calculateAgeFromBirthday', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 5, 11)); // 2026-06-11
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('calculates age for a past birthday this year', () => {
    expect(calculateAgeFromBirthday('1990-04-30')).toBe(36);
  });

  test('calculates age for an upcoming birthday this year', () => {
    expect(calculateAgeFromBirthday('1990-12-31')).toBe(35);
  });

  test('returns null for year-less and invalid birthdays', () => {
    expect(calculateAgeFromBirthday('--04-30')).toBeNull();
    expect(calculateAgeFromBirthday('')).toBeNull();
    expect(calculateAgeFromBirthday('30.04.1990')).toBeNull();
  });

  test('returns null for future birth years', () => {
    expect(calculateAgeFromBirthday('2030-01-01')).toBeNull();
  });
});

describe('calculateAgeAtDate', () => {
  test('computes age at a full date', () => {
    expect(calculateAgeAtDate('1990-05-01', '2020-01-15')).toBe(29);
  });

  test('is one lower before the birthday of that year', () => {
    expect(calculateAgeAtDate('1990-05-01', '2020-04-30')).toBe(29);
  });

  test('counts the birthday itself', () => {
    expect(calculateAgeAtDate('1990-05-01', '2020-05-01')).toBe(30);
  });

  test.each([
    ['a birthday without a year', '--05-01', '2020-01-15'],
    ['an empty birthday', '', '2020-01-15'],
    ['an empty target date', '1990-05-01', ''],
    ['a malformed target date', '1990-05-01', 'not-a-date'],
    ['a target date before the birthday', '1990-05-01', '1980-01-01'],
  ])('is null for %s', (_name, birthday, at) => {
    expect(calculateAgeAtDate(birthday, at)).toBeNull();
  });
});

describe('formatBirthdayForInputWithFormat', () => {
  test('converts ISO birthdays to the editable display format', () => {
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'eu')).toBe('30.04.1990');
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'us')).toBe('04/30/1990');
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'iso')).toBe('1990-04-30');
    expect(formatBirthdayForInputWithFormat('--04-30', 'eu')).toBe('30.04.');
    expect(formatBirthdayForInputWithFormat('--04-30', 'us')).toBe('04/30');
    expect(formatBirthdayForInputWithFormat('--04-30', 'iso')).toBe('04-30');
  });

  test('returns empty and unrecognized input unchanged', () => {
    expect(formatBirthdayForInputWithFormat('', 'eu')).toBe('');
    expect(formatBirthdayForInputWithFormat('whenever', 'eu')).toBe('whenever');
  });
});

describe('parseBirthdayInputWithFormat', () => {
  test('parses EU full dates and pads day/month', () => {
    expect(parseBirthdayInputWithFormat('30.04.1990', 'eu')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('1.4.1990', 'eu')).toBe('1990-04-01');
  });

  test('parses EU year-less dates with and without trailing dot', () => {
    expect(parseBirthdayInputWithFormat('30.04.', 'eu')).toBe('--04-30');
    expect(parseBirthdayInputWithFormat('30.04', 'eu')).toBe('--04-30');
  });

  test('parses US full and year-less dates', () => {
    expect(parseBirthdayInputWithFormat('04/30/1990', 'us')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('04/30', 'us')).toBe('--04-30');
  });

  test('accepts ISO input directly regardless of format', () => {
    expect(parseBirthdayInputWithFormat('1990-04-30', 'eu')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('--04-30', 'us')).toBe('--04-30');
  });

  test('returns empty string for empty input', () => {
    expect(parseBirthdayInputWithFormat('', 'eu')).toBe('');
    expect(parseBirthdayInputWithFormat('   ', 'eu')).toBe('');
  });

  test('rejects out-of-range and malformed input', () => {
    expect(parseBirthdayInputWithFormat('32.01.1990', 'eu')).toBeNull();
    expect(parseBirthdayInputWithFormat('30.13.1990', 'eu')).toBeNull();
    expect(parseBirthdayInputWithFormat('13/30/1990', 'us')).toBeNull();
    expect(parseBirthdayInputWithFormat('13/31/1990', 'us')).toBeNull();
    expect(parseBirthdayInputWithFormat('1990-13-31', 'iso')).toBeNull();
    expect(parseBirthdayInputWithFormat('hello', 'eu')).toBeNull();
    // Wrong separator for the active format
    expect(parseBirthdayInputWithFormat('04/30/1990', 'eu')).toBeNull();
  });
});

describe('autoFormatBirthdayInputWithFormat', () => {
  test('inserts separators while typing in EU format', () => {
    expect(autoFormatBirthdayInputWithFormat('3', '', 'eu')).toBe('3');
    expect(autoFormatBirthdayInputWithFormat('30', '3', 'eu')).toBe('30');
    expect(autoFormatBirthdayInputWithFormat('304', '30', 'eu')).toBe('30.4');
    expect(autoFormatBirthdayInputWithFormat('30.41', '30.4', 'eu')).toBe('30.41');
    expect(autoFormatBirthdayInputWithFormat('30.411', '30.41', 'eu')).toBe('30.41.1');
  });

  test('uses slashes in US format', () => {
    expect(autoFormatBirthdayInputWithFormat('043', '04', 'us')).toBe('04/3');
  });

  test('strips a trailing separator after deleting a digit', () => {
    expect(autoFormatBirthdayInputWithFormat('30.', '30.4', 'eu')).toBe('30');
  });

  test('keeps a manually typed separator at a boundary', () => {
    expect(autoFormatBirthdayInputWithFormat('30.', '30', 'eu')).toBe('30.');
  });

  test('leaves an ISO year untouched while it is being typed', () => {
    expect(autoFormatBirthdayInputWithFormat('1', '', 'iso')).toBe('1');
    expect(autoFormatBirthdayInputWithFormat('19', '1', 'iso')).toBe('19');
    expect(autoFormatBirthdayInputWithFormat('199', '19', 'iso')).toBe('199');
    expect(autoFormatBirthdayInputWithFormat('1990', '199', 'iso')).toBe('1990');
    expect(autoFormatBirthdayInputWithFormat('19900', '1990', 'iso')).toBe('1990-0');
    expect(autoFormatBirthdayInputWithFormat('1990-04', '1990-0', 'iso')).toBe('1990-04');
    expect(autoFormatBirthdayInputWithFormat('1990-043', '1990-04', 'iso')).toBe('1990-04-3');
    expect(autoFormatBirthdayInputWithFormat('1990-04-30', '1990-04-3', 'iso')).toBe('1990-04-30');
  });

  test('preserves manually typed ISO separators, including year-less MM-DD', () => {
    expect(autoFormatBirthdayInputWithFormat('04-', '04', 'iso')).toBe('04-');
    expect(autoFormatBirthdayInputWithFormat('04-3', '04-', 'iso')).toBe('04-3');
    expect(autoFormatBirthdayInputWithFormat('04-30', '04-3', 'iso')).toBe('04-30');
    expect(autoFormatBirthdayInputWithFormat('1990-', '1990', 'iso')).toBe('1990-');
    expect(autoFormatBirthdayInputWithFormat('1990-04-', '1990-04', 'iso')).toBe('1990-04-');
  });

  test('strips a trailing ISO separator after deleting a digit', () => {
    expect(autoFormatBirthdayInputWithFormat('1990-', '1990-0', 'iso')).toBe('1990');
  });
});
