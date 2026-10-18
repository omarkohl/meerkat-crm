import { apiFetch, API_BASE_URL, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';
import type { components } from '../types/generated/api';

export type Webhook = components['schemas']['WebhookResponse'];
export type WebhookCreateResponse = components['schemas']['WebhookCreatedResponse'];
export type WebhookInput = components['schemas']['WebhookBody'];

export interface WebhookDelivery extends Omit<components['schemas']['DeliveryResponse'], 'status'> {
  /** pending: waiting for a (re)try; delivered; dead: gave up after the last try. */
  status: 'pending' | 'delivered' | 'dead';
}

export async function getWebhooks(): Promise<Webhook[]> {
  const response = await apiFetch(`${API_BASE_URL}/webhooks`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to load webhooks.');
  return data?.['webhooks'] || [];
}

export async function createWebhook(input: WebhookInput): Promise<WebhookCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/webhooks`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  return handleResponse(response, 'Unable to create webhook.') as Promise<WebhookCreateResponse>;
}

export async function updateWebhook(id: number, input: WebhookInput): Promise<Webhook> {
  const response = await apiFetch(`${API_BASE_URL}/webhooks/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  return handleResponse(response, 'Unable to update webhook.') as Promise<Webhook>;
}

export async function deleteWebhook(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/webhooks/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to delete webhook.');
}

export async function testWebhook(id: number): Promise<{ delivery: WebhookDelivery }> {
  const response = await apiFetch(`${API_BASE_URL}/webhooks/${id}/test`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  return handleResponse(response, 'Unable to test webhook.') as Promise<{ delivery: WebhookDelivery }>;
}

export async function getWebhookDeliveries(id: number): Promise<WebhookDelivery[]> {
  const response = await apiFetch(`${API_BASE_URL}/webhooks/${id}/deliveries`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to load deliveries.');
  return data?.['deliveries'] || [];
}
