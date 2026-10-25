// Export API functions
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export function filenameFromDisposition(header: string | null, defaultFilename: string): string {
  if (!header) return defaultFilename;
  const encoded = /filename\*=UTF-8''([^;]+)/i.exec(header)?.[1];
  if (encoded) {
    try {
      return decodeURIComponent(encoded.trim());
    } catch {
      return plainFilename(header) ?? defaultFilename;
    }
  }
  return plainFilename(header) ?? defaultFilename;
}

function plainFilename(header: string): string | undefined {
  const captured = /filename=([^;]+)/.exec(header)?.[1];
  return captured ? captured.replace(/"/g, '').trim() : undefined;
}

/**
 * Helper function to download a file from an API response
 */
export async function downloadFileFromResponse(response: Response, defaultFilename: string): Promise<void> {
  const filename = filenameFromDisposition(response.headers.get('Content-Disposition'), defaultFilename);

  // Get the blob data
  const blob = await response.blob();

  // Create a download link and trigger download
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  window.URL.revokeObjectURL(url);
}

/**
 * Export all user data as CSV
 * Downloads a CSV file containing all contacts, activities, notes, relationships, and reminders
 */
export async function exportDataAsCsv(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/export`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await parseErrorResponse(response);
    throw error;
  }

  await downloadFileFromResponse(response, 'meerkat-export.csv');
}

export async function exportContactAsVcf(contactId: number | string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${String(contactId)}/vcf`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await parseErrorResponse(response);
    throw error;
  }

  await downloadFileFromResponse(response, `contact-${String(contactId)}.vcf`);
}

/**
 * Export all contacts as VCF (vCard)
 * Downloads a VCF file containing all contacts with their photos
 */
export async function exportContactsAsVcf(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/export/vcf`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await parseErrorResponse(response);
    throw error;
  }

  await downloadFileFromResponse(response, 'meerkat-contacts.vcf');
}
