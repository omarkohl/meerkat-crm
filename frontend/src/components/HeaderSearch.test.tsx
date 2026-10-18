import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act, within } from '@testing-library/react';
import { Link, MemoryRouter, useLocation } from 'react-router-dom';
import i18n from '../i18n/config';
import HeaderSearch from './HeaderSearch';
import * as searchApi from '../api/search';

vi.mock('../api/search', () => ({ searchAll: vi.fn() }));

const seg = (text: string, match = false) => ({ text, match });
const none = { total: 0, items: [] };

const found = (name: string, id = 7): searchApi.SearchResults => ({
  query: name,
  contacts: {
    total: 1,
    items: [
      {
        id,
        firstname: name,
        lastname: '',
        nickname: '',
        name: [seg(name, true)],
        matched_field: 'name',
        matched_field_name: '',
        snippet: [],
      },
    ],
  },
  notes: none,
  activities: none,
});

const empty: searchApi.SearchResults = { query: '', contacts: none, notes: none, activities: none };

function Where() {
  const l = useLocation();
  return <div data-testid="where">{l.pathname + l.search}</div>;
}

const renderSearch = () =>
  render(
    <MemoryRouter>
      <HeaderSearch enabled />
      <Where />
    </MemoryRouter>,
  );

const type = (value: string) => {
  fireEvent.change(screen.getByRole('combobox'), { target: { value } });
};

const settle = async () => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(300);
  });
};

describe('HeaderSearch', () => {
  beforeEach(async () => {
    vi.resetAllMocks();
    vi.useFakeTimers();
    await i18n.changeLanguage('en');
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  test('searches only from two characters and after the typing paused @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(empty);
    renderSearch();

    type('a');
    await settle();
    expect(searchApi.searchAll).not.toHaveBeenCalled();

    type('ad');
    type('ada');
    await settle();
    expect(searchApi.searchAll).toHaveBeenCalledTimes(1);
    expect(searchApi.searchAll).toHaveBeenCalledWith('ada', expect.anything());
  });

  test('shows the groups with their totals, the marked match and the show-all option last @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue({
      ...found('Ada'),
      notes: {
        total: 4,
        items: [
          {
            id: 1,
            contact_id: 7,
            contact_name: 'Ada',
            date: '2026-01-01T00:00:00Z',
            snippet: [seg('met '), seg('Ada', true)],
          },
        ],
      },
    });
    renderSearch();

    type('ada');
    await settle();

    const list = screen.getByRole('listbox');
    expect(within(list).getByText('Contacts (1)')).toBeTruthy();
    expect(within(list).getByText('Notes (4)')).toBeTruthy();
    const options = within(list).getAllByRole('option');
    expect(within(options[0] as HTMLElement).getByText('Ada').tagName).toBe('MARK');
    expect(options.at(-1)?.textContent).toBe('Show all results for “ada”');
  });

  test('says when nothing was found and still offers all results @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(empty);
    renderSearch();

    type('zzz');
    await settle();

    expect(screen.getByText('No results for “zzz”')).toBeTruthy();
    expect(screen.getByRole('option', { name: 'Show all results for “zzz”' })).toBeTruthy();
  });

  test('a slow answer to an older search never replaces a newer one @022-US1', async () => {
    let answerOld: (r: searchApi.SearchResults) => void = () => {};
    vi.mocked(searchApi.searchAll)
      .mockImplementationOnce(() => new Promise((resolve) => (answerOld = resolve)))
      .mockResolvedValueOnce(found('Newer'));
    renderSearch();

    type('ol');
    await settle();
    type('new');
    await settle();
    await act(async () => {
      answerOld(found('Older'));
      await Promise.resolve();
    });

    expect(screen.getByText('Newer')).toBeTruthy();
    expect(screen.queryByText('Older')).toBeNull();
  });

  test('opens the chosen contact and, without a choice, the contact list for the term @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(found('Ada', 42));
    renderSearch();

    type('ada');
    await settle();
    fireEvent.click(screen.getByRole('option', { name: /Ada/ }));
    expect(screen.getByTestId('where').textContent).toBe('/contacts/42');

    type('lovelace');
    await settle();
    fireEvent.keyDown(screen.getByRole('combobox'), { key: 'Enter' });
    expect(screen.getByTestId('where').textContent).toBe('/contacts?search=lovelace');
  });

  test('searching again for the same term opens the list again @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue(empty);
    render(
      <MemoryRouter initialEntries={['/']}>
        <HeaderSearch enabled />
        <Where />
        <Link to="/elsewhere">elsewhere</Link>
      </MemoryRouter>,
    );

    type('anna');
    await settle();
    fireEvent.keyDown(screen.getByRole('combobox'), { key: 'Enter' });
    expect(screen.getByTestId('where').textContent).toBe('/contacts?search=anna');
    fireEvent.click(screen.getByRole('link', { name: 'elsewhere' }));
    expect(screen.getByTestId('where').textContent).toBe('/elsewhere');

    type('anna');
    await settle();
    fireEvent.keyDown(screen.getByRole('combobox'), { key: 'Enter' });
    expect(screen.getByTestId('where').textContent).toBe('/contacts?search=anna');
  });

  test('the no-results row never shows for an older answer @022-US1', async () => {
    let answer: (r: searchApi.SearchResults) => void = () => {};
    vi.mocked(searchApi.searchAll)
      .mockResolvedValueOnce(empty)
      .mockImplementationOnce(() => new Promise((resolve) => (answer = resolve)));
    renderSearch();

    type('zzz');
    await settle();
    expect(screen.getByText('No results for “zzz”')).toBeTruthy();
    type('zzzy');
    await settle();

    expect(screen.queryByText(/No results for/)).toBeNull();
    await act(async () => {
      answer({ ...empty, query: 'zzzy' });
      await Promise.resolve();
    });
    expect(screen.getByText('No results for “zzzy”')).toBeTruthy();
  });

  test('names the field a contact matched in, also a custom one @022-US1', async () => {
    const hit = (id: number, field: string, fieldName: string, value: string) => ({
      id,
      firstname: 'Ada',
      lastname: '',
      nickname: '',
      name: [seg('Ada')],
      matched_field: field as 'organization',
      matched_field_name: fieldName,
      snippet: [seg(value, true)],
    });
    vi.mocked(searchApi.searchAll).mockResolvedValue({
      ...empty,
      contacts: {
        total: 3,
        items: [
          hit(1, 'organization', '', 'Engines'),
          hit(2, 'custom_field', 'Favourite bird', 'Pelican'),
          hit(3, 'note', '', 'tea'),
        ],
      },
    });
    renderSearch();

    type('e');
    type('en');
    await settle();

    expect(screen.getByText(/^Organization:/)).toBeTruthy();
    expect(screen.getByText(/^Favourite bird:/)).toBeTruthy();
    expect(screen.getByText(/^CardDAV note:/)).toBeTruthy();
  });

  test('shows the match without a label for a field it does not know @022-US1', async () => {
    vi.mocked(searchApi.searchAll).mockResolvedValue({
      ...empty,
      contacts: {
        total: 1,
        items: [
          {
            id: 1,
            firstname: 'Ada',
            lastname: '',
            nickname: '',
            name: [seg('Ada')],
            matched_field: 'something_new' as 'note',
            matched_field_name: '',
            snippet: [seg('hit', true)],
          },
        ],
      },
    });
    renderSearch();

    type('hit');
    await settle();

    expect(screen.getByText('hit').tagName).toBe('MARK');
    expect(screen.queryByText(/Note:/)).toBeNull();
  });
});
