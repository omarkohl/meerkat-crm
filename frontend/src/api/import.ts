import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import type { components } from '../types/generated/api';

type Schemas = components['schemas'];

export type ColumnMapping = Required<Schemas['ColumnMapping']>;
export interface ImportUploadResponse extends Omit<Schemas['UploadResponse'], 'suggested_mappings'> {
  suggested_mappings: ColumnMapping[];
}

export interface DuplicateMatch extends Omit<Schemas['DuplicateMatch'], 'match_reason'> {
  match_reason: 'name' | 'email' | 'phone' | 'vcard_uid';
}

export interface ImportRowPreview extends Omit<
  Schemas['RowPreview'],
  'parsed_contact' | 'duplicate_match' | 'suggested_action'
> {
  parsed_contact: Record<string, string>;
  duplicate_match: DuplicateMatch | null;
  suggested_action: 'add' | 'skip' | 'update';
}

export interface ImportPreviewResponse extends Omit<Schemas['PreviewResponse'], 'rows'> {
  rows: ImportRowPreview[];
}

export interface RowImportAction extends Omit<Schemas['RowAction'], 'action'> {
  action: 'skip' | 'add' | 'update';
}

export interface SkippedRow extends Omit<Schemas['SkippedRow'], 'reason'> {
  reason: 'skipped' | 'invalid' | 'failed';
}

export interface ImportResult extends Omit<Schemas['ImportResult'], 'skipped_rows'> {
  skipped_rows: SkippedRow[];
}

export interface VCFPhotoStatus extends Omit<Schemas['PhotoStatusResponse'], 'phase'> {
  phase: 'importing_photos' | 'done';
}

// Contact fields that can be imported (mirrors backend models.ImportableContactFields)
export const IMPORTABLE_CONTACT_FIELDS = [
  // Name
  'firstname',
  'lastname',
  'middle_name',
  'prefix',
  'suffix',
  'nickname',
  'gender',
  // Dates
  'birthday',
  'anniversary',
  // Multi-value: email / phone
  'email',
  'email_label',
  'phone',
  'phone_label',
  // Multi-value: address parts
  'address_street',
  'address_city',
  'address_region',
  'address_postal',
  'address_country',
  'address_label',
  // Multi-value: web / IM
  'url',
  'url_label',
  'impp',
  'impp_label',
  // Organization
  'organization',
  'department',
  'job_title',
  'role',
  // Free-text
  'how_we_met',
  'food_preference',
  'work_information',
  'contact_information',
  'circles',
  'note',
] as const;

export const REPEATABLE_VALUE_FIELDS = new Set<string>(['email', 'phone', 'url', 'impp']);

// Human-readable labels for contact fields
export const CONTACT_FIELD_LABELS: Record<string, string> = {
  firstname: 'First Name',
  lastname: 'Last Name',
  middle_name: 'Middle Name',
  prefix: 'Name Prefix',
  suffix: 'Name Suffix',
  nickname: 'Nickname',
  gender: 'Gender',
  birthday: 'Birthday',
  anniversary: 'Anniversary',
  email: 'Email',
  email_label: 'Email – Type',
  phone: 'Phone',
  phone_label: 'Phone – Type',
  address_street: 'Address – Street',
  address_city: 'Address – City',
  address_region: 'Address – Region/State',
  address_postal: 'Address – Postal Code',
  address_country: 'Address – Country',
  address_label: 'Address – Type',
  url: 'Website',
  url_label: 'Website – Type',
  impp: 'IM / Social',
  impp_label: 'IM / Social – Type',
  organization: 'Organization',
  department: 'Department',
  job_title: 'Job Title',
  role: 'Role',
  how_we_met: 'How We Met',
  food_preference: 'Food Preferences',
  work_information: 'Work Information',
  contact_information: 'Contact Information',
  circles: 'Circles',
  note: 'Note',
};

// Upload a CSV file for import
export async function uploadCSVForImport(file: File): Promise<ImportUploadResponse> {
  const formData = new FormData();
  formData.append('file', file);

  const response = await apiFetch(`${API_BASE_URL}/contacts/import/upload`, {
    method: 'POST',
    body: formData,
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Get import preview with applied mappings
export async function getImportPreview(sessionId: string, mappings: ColumnMapping[]): Promise<ImportPreviewResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/import/preview`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      session_id: sessionId,
      mappings,
    }),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Confirm and execute the import (CSV)
export async function confirmImport(sessionId: string, actions: RowImportAction[]): Promise<ImportResult> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/import/confirm`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      session_id: sessionId,
      actions,
    }),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Upload a VCF file for import (returns preview directly, no mapping needed)
export async function uploadVCFForImport(file: File): Promise<ImportPreviewResponse> {
  const formData = new FormData();
  formData.append('file', file);

  const response = await apiFetch(`${API_BASE_URL}/contacts/import/vcf/upload`, {
    method: 'POST',
    body: formData,
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Confirm and execute VCF import (with photo processing)
export async function confirmVCFImport(sessionId: string, actions: RowImportAction[]): Promise<ImportResult> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/import/vcf/confirm`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      session_id: sessionId,
      actions,
    }),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Progress of the photos that a confirmed VCF import loads in the background
export async function getVCFImportStatus(sessionId: string): Promise<VCFPhotoStatus> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/import/vcf/status?session_id=${encodeURIComponent(sessionId)}`,
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return (await response.json()) as VCFPhotoStatus;
}
