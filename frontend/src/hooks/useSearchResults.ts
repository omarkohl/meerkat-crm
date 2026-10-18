import { useEffect, useRef, useState } from 'react';
import { searchAll, SearchKind, SearchResults } from '../api/search';

export type { SearchKind, SearchResults, ContactResult, NoteResult, ActivityResult } from '../api/search';

export const MIN_CHARS = 2;
export const ALL_LIMIT = 10;
export const PAGE_SIZE = 25;
export const MAX_TERM = 200;
// The API refuses an offset beyond 1,000,000.
export const MAX_PAGE = Math.floor(1_000_000 / PAGE_SIZE);

export type Load<T> = { state: 'idle' | 'loading' | 'error' } | { state: 'ready'; data: T };

const idle = { state: 'idle' } as const;

/**
 * The results page's data: an overview (the counts and the first results of every group) for a
 * term, and for a group tab that group's page. Older answers never replace newer ones.
 */
export function useSearchResults(term: string, tab: 'all' | SearchKind, page: number) {
  const searchable = term.length >= MIN_CHARS;
  const [overview, setOverview] = useState<Load<SearchResults>>(idle);
  const [paged, setPaged] = useState<Load<SearchResults>>(idle);
  const [attempt, setAttempt] = useState(0);
  const latestOverview = useRef(0);
  const latestPaged = useRef(0);

  useEffect(() => {
    latestOverview.current += 1;
    const request = latestOverview.current;
    if (!searchable) {
      setOverview(idle);
      return;
    }
    setOverview({ state: 'loading' });
    searchAll(term, { limit: ALL_LIMIT })
      .then((data) => {
        if (request === latestOverview.current) setOverview({ state: 'ready', data });
      })
      .catch((err: unknown) => {
        console.error('Search error:', err);
        if (request === latestOverview.current) setOverview({ state: 'error' });
      });
  }, [term, searchable, attempt]);

  useEffect(() => {
    latestPaged.current += 1;
    const request = latestPaged.current;
    if (!searchable || tab === 'all') {
      setPaged(idle);
      return;
    }
    setPaged({ state: 'loading' });
    searchAll(term, { type: tab, limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE })
      .then((data) => {
        if (request === latestPaged.current) setPaged({ state: 'ready', data });
      })
      .catch((err: unknown) => {
        console.error('Search error:', err);
        if (request === latestPaged.current) setPaged({ state: 'error' });
      });
  }, [term, searchable, tab, page, attempt]);

  return {
    searchable,
    overview,
    paged,
    retry: () => {
      setAttempt((n) => n + 1);
    },
  };
}
