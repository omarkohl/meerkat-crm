import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Link, MemoryRouter } from 'react-router-dom';
import i18n from './i18n/config';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import ActivitiesPage from './ActivitiesPage';

vi.mock('./hooks/useActivities', () => ({
  useActivities: () => ({
    activities: [],
    total: 0,
    page: 1,
    limit: 25,
    loading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

const renderPage = (url: string) =>
  render(
    <MemoryRouter initialEntries={[url]}>
      <SnackbarProvider>
        <DateFormatProvider>
          <ActivitiesPage />
          <Link to="/activities?search=second">second</Link>
        </DateFormatProvider>
      </SnackbarProvider>
    </MemoryRouter>,
  );

describe('ActivitiesPage search from the address', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
  });

  test('starts with the term of the address and follows a new one @022-US1', () => {
    renderPage('/activities?search=first');
    expect(screen.getByRole('textbox', { name: 'Search activities...' })).toHaveProperty('value', 'first');

    fireEvent.click(screen.getByRole('link', { name: 'second' }));

    expect(screen.getByRole('textbox', { name: 'Search activities...' })).toHaveProperty('value', 'second');
  });
});
