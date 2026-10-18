// Admin API calls for user management
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import type { User, UsersListResponse, UserUpdateInput } from '../types';
import type { components } from '../types/generated/api';
import type { WithoutNull } from '../types/utils';

// Get current authenticated user's information
export async function getCurrentUser(): Promise<User> {
  const response = await apiFetch(`${API_BASE_URL}/users/me`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Get paginated list of all users (admin only)
export async function getUsers(page: number = 1, limit: number = 25): Promise<UsersListResponse> {
  const params = new URLSearchParams({
    page: page.toString(),
    limit: limit.toString(),
  });

  const response = await apiFetch(`${API_BASE_URL}/admin/users?${params}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Get a single user by ID (admin only)
export async function getUserById(id: number): Promise<User> {
  const response = await apiFetch(`${API_BASE_URL}/admin/users/${id}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Update a user (admin only)
export async function updateUser(id: number, data: UserUpdateInput): Promise<User> {
  const response = await apiFetch(`${API_BASE_URL}/admin/users/${id}`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Trigger reminder emails manually (admin only)
export async function triggerReminders(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/admin/trigger-reminders`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}

/**
 * Delete a user (admin only)
 */
export async function deleteUser(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/admin/users/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}

export type SystemStatus = WithoutNull<components['schemas']['SystemStatusOutputBody']>;

// Operational warnings for admins (admin only)
export async function getSystemStatus(): Promise<SystemStatus> {
  const response = await apiFetch(`${API_BASE_URL}/admin/system-status`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// The dismissal is tied to the situation it was made in: a different set of risky features
// or a much bigger instance shows the banner again.
export function ssrfBannerSignature(status: { setting: string; features?: string[]; user_count: number }): string {
  const n = status.user_count;
  const bucket = n <= 2 ? '2' : n <= 5 ? '3-5' : n <= 10 ? '6-10' : n <= 50 ? '11-50' : '51+';
  return [status.setting, [...(status.features ?? [])].sort().join(','), bucket].join('|');
}

export type AuditEntry = components['schemas']['AuditEntryResponse'];

export type AuditLogPage = WithoutNull<Omit<components['schemas']['AuditLogOutputBody'], 'entries'>> & {
  entries: AuditEntry[];
};

export interface AuditLogQuery {
  page: number;
  limit: number;
  event?: string;
  username?: string;
}

// Security events, newest first (admin only)
export async function getAuditLog(query: AuditLogQuery): Promise<AuditLogPage> {
  const params = new URLSearchParams({ page: String(query.page), limit: String(query.limit) });
  if (query.event) params.set('event', query.event);
  if (query.username) params.set('username', query.username);

  const response = await apiFetch(`${API_BASE_URL}/admin/audit-log?${params}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
  return response.json();
}
