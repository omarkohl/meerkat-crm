import { apiFetch, API_BASE_URL, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';
import type { components } from '../types/generated/api';

export type IcsFeedStatus = components['schemas']['FeedStatusResponse'];
export type IcsFeedCreateResponse = components['schemas']['FeedCreatedResponse'];

export async function getIcsFeed(): Promise<IcsFeedStatus> {
  const response = await apiFetch(`${API_BASE_URL}/ics-feed`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to load calendar feed.');
  return {
    enabled: !!data?.['enabled'],
    created_at: data?.['created_at'] ?? null,
    last_fetched_at: data?.['last_fetched_at'] ?? null,
  };
}

/** Creates the feed, or replaces its token if one exists (the old URL stops working). */
export async function createIcsFeed(): Promise<IcsFeedCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/ics-feed`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to create calendar feed.');
  return data as IcsFeedCreateResponse;
}

export async function revokeIcsFeed(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/ics-feed`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to revoke calendar feed.');
}

/** Absolute, subscribable URL for a feed token (the API base may be relative). */
export function buildIcsFeedUrl(token: string): string {
  return new URL(`${API_BASE_URL}/ics/${token}.ics`, window.location.origin).href;
}
