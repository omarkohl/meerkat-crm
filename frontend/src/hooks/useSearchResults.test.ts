import { describe, test, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import * as searchApi from '../api/search';
import { useSearchResults } from './useSearchResults';

vi.mock('../api/search', () => ({ searchAll: vi.fn() }));

const answer = (query: string): searchApi.SearchResults => ({
  query,
  contacts: { total: 1, items: [] },
  notes: { total: 0, items: [] },
  activities: { total: 0, items: [] },
});

describe('useSearchResults', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  test('a slow answer for an older term never replaces the newer one @022-US1', async () => {
    let old: (r: searchApi.SearchResults) => void = () => {};
    vi.mocked(searchApi.searchAll)
      .mockImplementationOnce(() => new Promise((resolve) => (old = resolve)))
      .mockResolvedValueOnce(answer('newer'));
    const { result, rerender } = renderHook(({ term }) => useSearchResults(term, 'all', 1), {
      initialProps: { term: 'old' },
    });

    rerender({ term: 'newer' });
    await waitFor(() => {
      expect(result.current.overview).toEqual({ state: 'ready', data: answer('newer') });
    });
    await act(async () => {
      old(answer('old'));
      await Promise.resolve();
    });

    expect(result.current.overview).toEqual({ state: 'ready', data: answer('newer') });
  });

  test('the same holds for a group page @022-US1', async () => {
    let first: (r: searchApi.SearchResults) => void = () => {};
    vi.mocked(searchApi.searchAll).mockImplementation((_term, options) =>
      options?.type
        ? options.offset === 0
          ? new Promise((resolve) => (first = resolve))
          : Promise.resolve(answer('page2'))
        : Promise.resolve(answer('overview')),
    );
    const { result, rerender } = renderHook(({ page }) => useSearchResults('ad', 'contacts', page), {
      initialProps: { page: 1 },
    });

    rerender({ page: 2 });
    await waitFor(() => {
      expect(result.current.paged).toEqual({ state: 'ready', data: answer('page2') });
    });
    await act(async () => {
      first(answer('page1'));
      await Promise.resolve();
    });

    expect(result.current.paged).toEqual({ state: 'ready', data: answer('page2') });
  });
});
