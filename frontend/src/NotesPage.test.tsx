import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Link, MemoryRouter } from 'react-router-dom';
import i18n from './i18n/config';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import NotesPage from './NotesPage';

vi.mock('./hooks/useNotes', () => ({
  useNotes: () => ({ notes: [], total: 0, page: 1, limit: 25, loading: false, error: null, refetch: vi.fn() }),
}));

const renderPage = (url: string) =>
  render(
    <MemoryRouter initialEntries={[url]}>
      <SnackbarProvider>
        <DateFormatProvider>
          <NotesPage />
          <Link to="/notes?search=second">second</Link>
        </DateFormatProvider>
      </SnackbarProvider>
    </MemoryRouter>,
  );

describe('NotesPage search from the address', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
  });

  test('starts with the term of the address and follows a new one @022-US1', () => {
    renderPage('/notes?search=first');
    expect(screen.getByRole('textbox', { name: 'Search notes...' })).toHaveProperty('value', 'first');

    fireEvent.click(screen.getByRole('link', { name: 'second' }));

    expect(screen.getByRole('textbox', { name: 'Search notes...' })).toHaveProperty('value', 'second');
  });
});
