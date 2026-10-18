import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import i18n from './i18n/config';
import { DateFormatProvider } from './DateFormatProvider';
import SearchPage from './SearchPage';
import * as searchApi from './api/search';

vi.mock('./api/search', () => ({ searchAll: vi.fn() }));

const seg = (text: string, match = false) => ({ text, match });
const contact = (id: number, name: string) => ({
  id,
  firstname: name,
  lastname: '',
  nickname: '',
  name: [seg(name.slice(0, 2), true), seg(name.slice(2))],
  matched_field: 'name' as const,
  matched_field_name: '',
  snippet: [],
});
const note = (id: number, contactId: number | null) => ({
  id,
  contact_id: contactId,
  contact_name: contactId ? 'Ada Lovelace' : '',
  date: '2026-01-05T00:00:00Z',
  snippet: [seg('we met '), seg('Ada', true)],
});
const activity = (id: number) => ({
  id,
  title: [seg('Dinner with '), seg('Ada', true)],
  date: '2026-02-01T00:00:00Z',
  snippet: [],
  contacts: ['Ada Lovelace', 'Grace Hopper'],
});

const results = (over: Partial<searchApi.SearchResults> = {}): searchApi.SearchResults => ({
  query: 'ad',
  contacts: { total: 12, items: [contact(1, 'Ada'), contact(2, 'Adam')] },
  notes: { total: 1, items: [note(5, 7)] },
  activities: { total: 1, items: [activity(9)] },
  ...over,
});

function Where() {
  const l = useLocation();
  return <div data-testid="where">{l.pathname + l.search}</div>;
}

const renderPage = (url: string) =>
  render(
    <MemoryRouter initialEntries={[url]}>
      <DateFormatProvider>
        <SearchPage />
        <Where />
      </DateFormatProvider>
    </MemoryRouter>,
  );

describe('SearchPage', () => {
  beforeEach(async () => {
    vi.resetAllMocks();
    window.localStorage.setItem('dateFormat', 'iso');
    await i18n.changeLanguage('en');
  });

  test('fills the field with the term from the address and shows the groups with counts @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(results());
    renderPage('/search?q=ad');

    expect(await screen.findByRole('heading', { name: 'Contacts (12)' })).toBeTruthy();
    expect(searchApi.searchAll).toHaveBeenCalledWith('ad', { limit: 10 });
    expect(screen.getByRole('searchbox', { name: 'Search' })).toHaveProperty('value', 'ad');
    expect(screen.getByRole('tab', { name: 'All (14)' })).toBeTruthy();
    expect(screen.getByRole('tab', { name: 'Notes (1)' })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Notes (1)' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Ada' }).querySelector('mark')?.textContent).toBe('Ad');
    expect(screen.getByRole('button', { name: 'Show all 12' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Show all 1' })).toBeNull();
  });

  test('a note links to its contact, or to the notes page with the term when it has none; an activity to the activities page @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(
      results({ notes: { total: 2, items: [note(5, 7), note(6, null)] } }),
    );
    renderPage('/search?q=ad');

    await screen.findByRole('heading', { name: 'Notes (2)' });
    const hrefs = screen.getAllByRole('link').map((a) => a.getAttribute('href'));
    expect(hrefs).toContain('/contacts/7');
    expect(hrefs).toContain('/notes?search=ad');
    expect(hrefs).toContain('/activities?search=ad');
    expect(hrefs).toContain('/contacts/1');
  });

  test('Show all switches to that tab, which pages its group with the API @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockImplementation((_q, options) =>
      Promise.resolve(
        options?.type === 'contacts'
          ? results({ contacts: { total: 60, items: [contact(30, 'Adrian')] } })
          : results({ contacts: { total: 60, items: [contact(1, 'Ada')] } }),
      ),
    );
    renderPage('/search?q=ad');
    fireEvent.click(await screen.findByRole('button', { name: 'Show all 60' }));

    expect(await screen.findByRole('link', { name: 'Adrian' })).toBeTruthy();
    expect(screen.getByTestId('where').textContent).toBe('/search?q=ad&tab=contacts');
    expect(screen.getByRole('tab', { name: 'Contacts (60)', selected: true })).toBeTruthy();
    expect(document.activeElement?.id).toBe('search-tab-contacts');
    expect(searchApi.searchAll).toHaveBeenCalledWith('ad', { type: 'contacts', limit: 25, offset: 0 });

    fireEvent.click(screen.getByRole('button', { name: 'Go to page 2' }));
    await waitFor(() => {
      expect(searchApi.searchAll).toHaveBeenCalledWith('ad', { type: 'contacts', limit: 25, offset: 25 });
    });
    expect(screen.getByTestId('where').textContent).toBe('/search?q=ad&tab=contacts&page=2');
  });

  test('a new term from the field goes into the address and searches again @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(results());
    renderPage('/search?q=ad&tab=notes');
    await screen.findByRole('tab', { name: 'Notes (1)', selected: true });

    fireEvent.change(screen.getByRole('searchbox', { name: 'Search' }), { target: { value: 'grace' } });
    fireEvent.submit(screen.getByRole('search'));

    await waitFor(() => {
      expect(searchApi.searchAll).toHaveBeenCalledWith('grace', { limit: 10 });
    });
    expect(screen.getByTestId('where').textContent).toBe('/search?q=grace');
  });

  test('says when nothing was found and when the term is too short @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue({
      query: 'zz',
      contacts: { total: 0, items: [] },
      notes: { total: 0, items: [] },
      activities: { total: 0, items: [] },
    });
    renderPage('/search?q=zz');
    expect(await screen.findByText('No results for “zz”')).toBeTruthy();
  });

  test('does not search below two characters @022-US1', () => {
    renderPage('/search?q=a');
    expect(screen.getByText('Type at least 2 characters to search.')).toBeTruthy();
    expect(searchApi.searchAll).not.toHaveBeenCalled();
  });

  test('shows an error with a retry that searches again @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce(results());
    vi.spyOn(console, 'error').mockImplementation(() => {});
    renderPage('/search?q=ad');

    const alert = await screen.findByRole('alert');
    expect(within(alert).getByText('The search failed. Please try again.')).toBeTruthy();
    fireEvent.click(within(alert).getByRole('button', { name: 'Try again' }));

    expect(await screen.findByRole('heading', { name: 'Contacts (12)' })).toBeTruthy();
  });

  test('an activity shows its date and contacts @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(results());
    renderPage('/search?q=ad');

    expect(await screen.findByText(/2026-02-01 · Ada Lovelace, Grace Hopper/)).toBeTruthy();
  });

  test('the Contacts tab links to the contact list with the term @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(results());
    renderPage('/search?q=ad%20b&tab=contacts');

    const link = await screen.findByRole('link', { name: 'Show in the contact list' });
    expect(link.getAttribute('href')).toBe('/contacts?search=ad%20b');
  });

  test('a term over 200 characters is cut and a huge page number is clamped @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(results());
    renderPage(`/search?q=${'x'.repeat(300)}&tab=notes&page=99999999999`);

    await waitFor(() => {
      expect(searchApi.searchAll).toHaveBeenCalledWith('x'.repeat(200), { type: 'notes', limit: 25, offset: 999975 });
    });
  });

  test('a page beyond the last one shows the last page @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockImplementation((_q, options) =>
      Promise.resolve(results({ contacts: { total: 60, items: [contact(options?.offset ? 50 : 1, 'Adrian')] } })),
    );
    renderPage('/search?q=ad&tab=contacts&page=9');

    await waitFor(() => {
      expect(screen.getByTestId('where').textContent).toBe('/search?q=ad&tab=contacts&page=3');
    });
    await waitFor(() => {
      expect(searchApi.searchAll).toHaveBeenCalledWith('ad', { type: 'contacts', limit: 25, offset: 50 });
    });
  });
});
