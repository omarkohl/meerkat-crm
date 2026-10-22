import { describe, test, expect } from 'vitest';
import { useState } from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import ActivityDateFields from './ActivityDateFields';

const Harness = ({ date, endDate }: { date: string; endDate: string }) => {
  const [dates, setDates] = useState({ date, endDate });
  return (
    <DateFormatProvider>
      <ActivityDateFields {...dates} onChange={setDates} />
    </DateFormatProvider>
  );
};

const start = () => screen.getByRole<HTMLInputElement>('textbox', { name: 'Date' });
const end = () => screen.getByRole<HTMLInputElement>('textbox', { name: 'End date (optional)' });

// 2026-03-12 shows as 12.03.2026 in the default date format
describe('ActivityDateFields', () => {
  test('the end date starts empty and takes a later day @005-US6', () => {
    render(<Harness date="2026-03-12" endDate="" />);

    expect(end().value).toBe('');
    fireEvent.change(end(), { target: { value: '15.03.2026' } });
    expect(end().value).toBe('15.03.2026');
    expect(screen.queryByText('The end date must not be before the date')).toBeNull();
  });

  test('an end date before the start is flagged @005-US6', () => {
    render(<Harness date="2026-03-12" endDate="" />);

    fireEvent.change(end(), { target: { value: '10.03.2026' } });

    expect(screen.getByText('The end date must not be before the date')).toBeTruthy();
  });

  test('moving the start past the end moves the end along @005-US6', () => {
    render(<Harness date="2026-03-12" endDate="2026-03-15" />);

    fireEvent.change(start(), { target: { value: '20.03.2026' } });

    expect(end().value).toBe('20.03.2026');
  });

  test('moving the start within the range keeps the end @005-US6', () => {
    render(<Harness date="2026-03-12" endDate="2026-03-15" />);

    fireEvent.change(start(), { target: { value: '13.03.2026' } });

    expect(end().value).toBe('15.03.2026');
  });
});
