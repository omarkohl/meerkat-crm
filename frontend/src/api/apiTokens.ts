import { apiFetch, API_BASE_URL, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';
import type { components } from '../types/generated/api';

export type ApiToken = components['schemas']['APITokenResponse'];
export type ApiTokenCreateResponse = components['schemas']['APITokenCreatedResponse'];

export interface ApiTokensListResponse extends Omit<components['schemas']['APITokenListOutputBody'], 'tokens'> {
  tokens: ApiToken[];
}

export async function getApiTokens(): Promise<ApiTokensListResponse> {
  const response = await apiFetch(`${API_BASE_URL}/api-tokens`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to load API tokens.');
  return { tokens: data?.['tokens'] || [] };
}

export async function createApiToken(name: string): Promise<ApiTokenCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/api-tokens`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ name }),
  });
  const data = await handleResponse(response, 'Unable to create API token.');
  return data as ApiTokenCreateResponse;
}

export async function revokeApiToken(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/api-tokens/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to revoke API token.');
}
