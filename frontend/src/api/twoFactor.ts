import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import type { components } from '../types/generated/api';

export type TwoFactorStatus = components['schemas']['TwoFactorStatusOutputBody'];
export type TwoFactorSetup = components['schemas']['TwoFactorSetupOutputBody'];
/** Password plus one authenticator code or recovery code. */
export type TwoFactorConfirmation = components['schemas']['ConfirmTwoFactorBody'];
type CodesBody = components['schemas']['TwoFactorCodesOutputBody'];

async function call<T>(path: string, method: string, body?: unknown): Promise<T> {
  const response = await apiFetch(`${API_BASE_URL}${path}`, {
    method,
    headers: getAuthHeaders(),
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
  return response.json();
}

export const getTwoFactorStatus = () => call<TwoFactorStatus>('/users/me/2fa', 'GET');

/** Creates a pending secret; two-factor stays off until enableTwoFactor succeeds. */
export const setupTwoFactor = () => call<TwoFactorSetup>('/users/me/2fa/setup', 'POST');

/** Returns the recovery codes. They are shown once and never again. */
export async function enableTwoFactor(password: string, code: string): Promise<string[]> {
  const data = await call<CodesBody>('/users/me/2fa/enable', 'POST', {
    password,
    code,
  } satisfies components['schemas']['EnableTwoFactorBody']);
  return data.recovery_codes ?? [];
}

export async function disableTwoFactor(confirmation: TwoFactorConfirmation): Promise<void> {
  await call('/users/me/2fa/disable', 'POST', confirmation);
}

export async function regenerateRecoveryCodes(confirmation: TwoFactorConfirmation): Promise<string[]> {
  const data = await call<CodesBody>('/users/me/2fa/recovery-codes', 'POST', confirmation);
  return data.recovery_codes ?? [];
}

/** Admin only: switches off a user's two-factor and signs the user out everywhere. */
export async function resetUserTwoFactor(userId: number): Promise<void> {
  await call(`/admin/users/${userId}/2fa/reset`, 'POST');
}
