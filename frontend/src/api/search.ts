import { apiFetch, API_BASE_URL, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';
import type { components } from '../types/generated/api';

export type Segment = components['schemas']['SegmentResponse'];
export type ContactResult = components['schemas']['ContactResult'];
export type NoteResult = components['schemas']['NoteResult'];
export type ActivityResult = components['schemas']['ActivityResult'];

export interface SearchGroup<T> {
  total: number;
  items: T[];
}

export interface SearchResults {
  query: string;
  contacts: SearchGroup<ContactResult>;
  notes: SearchGroup<NoteResult>;
  activities: SearchGroup<ActivityResult>;
}

type SearchBody = components['schemas']['SearchOutputBody'];

const group = <T>(g: { total: number; items: T[] | null } | undefined): SearchGroup<T> => ({
  total: g?.total ?? 0,
  items: g?.items ?? [],
});

export async function searchAll(query: string, options: { limit?: number } = {}): Promise<SearchResults> {
  const params = new URLSearchParams({ q: query });
  if (options.limit) params.set('limit', String(options.limit));
  const response = await apiFetch(`${API_BASE_URL}/search?${params.toString()}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = (await handleResponse(response, 'Unable to search.')) as SearchBody;
  return {
    query: data.query,
    contacts: group(data.contacts),
    notes: group(data.notes),
    activities: group(data.activities),
  };
}
