import { apiFetch, API_BASE_URL, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';
import type { components, operations } from '../types/generated/api';

export type TrashedContact = components['schemas']['TrashedContactResponse'];
type TrashListBody = operations['list-trash']['responses']['200']['content']['application/json'];

export interface TrashResponse extends Omit<TrashListBody, 'contacts'> {
  contacts: TrashedContact[];
}

export async function getTrash(): Promise<TrashResponse> {
  const response = await apiFetch(`${API_BASE_URL}/trash/contacts`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to load the trash.');
  return { contacts: data?.['contacts'] || [], retention_days: data?.['retention_days'] ?? 30 };
}

export async function restoreTrashedContact(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/trash/contacts/${id}/restore`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to restore the contact.');
}

export async function purgeTrashedContact(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/trash/contacts/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to delete the contact permanently.');
}

/** Permanently deletes everything in the trash and returns how many contacts were removed. */
export async function emptyTrash(): Promise<number> {
  const response = await apiFetch(`${API_BASE_URL}/trash/contacts`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to empty the trash.');
  return data?.['purged'] ?? 0;
}
