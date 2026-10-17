// User settings and preferences API calls
import { apiFetch, API_BASE_URL, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';
import type { components, operations } from '../types/generated/api';

type Body<Op extends keyof operations> = operations[Op] extends {
  responses: { 200: { content: { 'application/json': infer B } } };
}
  ? B
  : never;
type LanguageSaved = Body<'update-language'>;
type DateFormatSaved = Body<'update-date-format'>;
type CustomFieldsBody = Body<'get-custom-field-names'>;
type CustomFieldsSaved = Body<'update-custom-field-names'>;
type EnabledFieldsBody = Body<'get-enabled-contact-fields'>;
type EnabledFieldsSaved = Body<'update-enabled-contact-fields'>;
type LanguageRequest = components['schemas']['LanguageBody'];
type DateFormatRequest = components['schemas']['DateFormatBody'];
type CustomFieldsRequest = components['schemas']['CustomFieldsBody'];
type EnabledFieldsRequest = components['schemas']['EnabledFieldsBody'];

export async function updateLanguage(language: string): Promise<string> {
  const response = await apiFetch(`${API_BASE_URL}/users/language`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify({ language } satisfies LanguageRequest),
  });

  const data = (await handleResponse(response, 'Unable to update language.')) as LanguageSaved | undefined;
  return data?.message || 'Language updated successfully.';
}

export async function updateDateFormat(dateFormat: string): Promise<string> {
  const response = await apiFetch(`${API_BASE_URL}/users/date-format`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify({ date_format: dateFormat } satisfies DateFormatRequest),
  });

  const data = (await handleResponse(response, 'Unable to update date format.')) as DateFormatSaved | undefined;
  return data?.message || 'Date format updated successfully.';
}

export async function getCustomFieldNames(): Promise<string[]> {
  const response = await apiFetch(`${API_BASE_URL}/users/custom-fields`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  const data = (await handleResponse(response, 'Unable to get custom field names.')) as CustomFieldsBody | undefined;
  return data?.custom_field_names || [];
}

export async function updateCustomFieldNames(names: string[]): Promise<string[]> {
  const response = await apiFetch(`${API_BASE_URL}/users/custom-fields`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify({ names } satisfies CustomFieldsRequest),
  });

  const data = (await handleResponse(response, 'Unable to update custom field names.')) as
    CustomFieldsSaved | undefined;
  return data?.custom_field_names || names;
}

// Returns the user's enabled extended contact fields, or null if never configured
// (in which case the caller should apply DEFAULT_ENABLED_CONTACT_FIELDS).
export async function getEnabledContactFields(): Promise<string[] | null> {
  const response = await apiFetch(`${API_BASE_URL}/users/enabled-contact-fields`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  const data = (await handleResponse(response, 'Unable to get enabled contact fields.')) as
    EnabledFieldsBody | undefined;
  return data?.enabled_contact_fields ?? null;
}

export async function updateEnabledContactFields(fields: string[]): Promise<string[]> {
  const response = await apiFetch(`${API_BASE_URL}/users/enabled-contact-fields`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify({ fields } satisfies EnabledFieldsRequest),
  });

  const data = (await handleResponse(response, 'Unable to update enabled contact fields.')) as
    EnabledFieldsSaved | undefined;
  return data?.enabled_contact_fields || fields;
}
