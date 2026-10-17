import { apiFetch, API_BASE_URL, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';
import type { components, operations } from '../types/generated/api';

export type Session = components['schemas']['SessionResponse'];
type SessionListBody = operations['list-sessions']['responses']['200']['content']['application/json'];

export async function getSessions(): Promise<Session[]> {
  const response = await apiFetch(`${API_BASE_URL}/sessions`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = (await handleResponse(response, 'Unable to load sessions.')) as SessionListBody | undefined;
  return data?.sessions ?? [];
}

export async function revokeSession(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/sessions/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to sign the session out.');
}

/** Revokes every session, this one included. */
export async function signOutEverywhere(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/sessions/revoke-all`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to sign out everywhere.');
}
