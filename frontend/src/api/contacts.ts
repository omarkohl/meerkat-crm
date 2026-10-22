// Contact-related API calls
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import type { components, operations } from '../types/generated/api';
import type { WithoutNull } from '../types/utils';
import type { ImageRef } from './images';

export type { ImageRef };
import type { Crop } from '../utils/faceCrop';

// Types of the contacts module's operations, generated from the OpenAPI document (docs/openapi.json).
export type ContactResponse = components['schemas']['ContactResponse'];
type CirclesResponse = operations['list-circles']['responses']['200']['content']['application/json'];

export type ContactValue = components['schemas']['ContactValue'];
export type ContactAddress = components['schemas']['ContactAddress'];
export type Birthday = components['schemas']['BirthdayResponse'];
type ContactListItem = components['schemas']['ContactListItem'];
type RandomResponse = operations['random-contacts']['responses']['200']['content']['application/json'];
type BirthdaysResponse = operations['upcoming-birthdays']['responses']['200']['content']['application/json'];
type ContactsListResponse = operations['list-contacts']['responses']['200']['content']['application/json'];

// A contact as the list and detail endpoints return it: with `fields` only some values are filled.
type ContactValues = WithoutNull<Omit<ContactListItem, 'notes' | 'activities' | 'reminders' | 'relationships'>>;
export type Contact = Partial<ContactValues> & Pick<ContactValues, 'ID' | 'firstname' | 'lastname'>;
export type ContactInput = Partial<components['schemas']['ContactBody']>;

export interface ContactsResponse extends Omit<ContactsListResponse, 'contacts'> {
  contacts: Contact[];
}

export interface GetContactsParams {
  page?: number;
  limit?: number;
  search?: string;
  searchAll?: boolean;
  circle?: string;
  sort?: string;
  order?: string;
  includeArchived?: boolean;
  archived?: boolean;
}

// Get all contacts with pagination and filters
export async function getContacts(params: GetContactsParams): Promise<ContactsResponse> {
  const { page = 1, limit = 25, search = '', searchAll, circle = '', sort, order, includeArchived, archived } = params;

  const queryParams = new URLSearchParams({
    page: page.toString(),
    limit: limit.toString(),
  });

  if (search) queryParams.append('search', search);
  if (search && searchAll) queryParams.append('search_all', 'true');
  if (circle) queryParams.append('circle', circle);
  if (sort) queryParams.append('sort', sort);
  if (order) queryParams.append('order', order);
  if (includeArchived) queryParams.append('include_archived', 'true');
  if (archived !== undefined) queryParams.append('archived', archived.toString());

  queryParams.append('fields', 'ID,firstname,lastname,nickname,circles,photo_thumbnail,archived');

  const response = await apiFetch(`${API_BASE_URL}/contacts?${queryParams.toString()}`, { headers: getAuthHeaders() });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return (await response.json()) as ContactsResponse;
}

// Get single contact
export async function getContact(id: string | number, fields?: string[]): Promise<Contact> {
  let url = `${API_BASE_URL}/contacts/${id}`;
  if (fields && fields.length > 0) {
    url += `?fields=${fields.join(',')}`;
  }

  const response = await apiFetch(url, { headers: getAuthHeaders() });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Create contact
export async function createContact(data: ContactInput): Promise<Contact> {
  const response = await apiFetch(`${API_BASE_URL}/contacts`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  const result = await response.json();
  return result.contact || result;
}

// Update contact
export async function updateContact(id: string | number, data: ContactInput): Promise<Contact> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Delete contact
export async function deleteContact(id: string | number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}

// Get contact profile picture
export async function getContactProfilePicture(id: string | number, thumbnail: boolean = false): Promise<Blob | null> {
  const url = thumbnail
    ? `${API_BASE_URL}/contacts/${id}/profile_picture?thumbnail=true`
    : `${API_BASE_URL}/contacts/${id}/profile_picture`;
  const response = await apiFetch(url);

  if (!response.ok) {
    return null;
  }

  return response.blob();
}

export type ImageCrop = components['schemas']['ImageCrop'];

export async function setContactImages(id: number, imageIds: number[]): Promise<ImageRef[]> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${String(id)}/images`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify({ image_ids: imageIds }),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
  return (await response.json()) as ImageRef[];
}

export async function setMainImage(id: number, imageId: number, crop: ImageCrop): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${String(id)}/main_image`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify({ image_id: imageId, crop }),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}

// Appends an uploaded photo to the gallery. It becomes the main image, with the crop, when one is given.
export async function addContactPhoto(
  id: number,
  currentIds: number[],
  image: ImageRef,
  crop: Crop | null,
): Promise<void> {
  await setContactImages(id, [...currentIds, image.id]);
  if (crop) await setMainImage(id, image.id, crop);
}

// Get all circles
export async function getCircles(): Promise<string[]> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/circles`, { headers: getAuthHeaders() });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  const data = (await response.json()) as CirclesResponse | null;
  // The backend returns the array directly (null when there are no circles)
  return Array.isArray(data) ? data : [];
}

// Get random contacts (returns 5 contacts)
export async function getRandomContacts(): Promise<Contact[]> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/random`, { headers: getAuthHeaders() });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  const data = (await response.json()) as RandomResponse;
  return data.contacts as Contact[];
}

// Get upcoming birthdays (returns up to 10 birthdays for contacts and relationships)
export async function getUpcomingBirthdays(): Promise<Birthday[]> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/birthdays`, { headers: getAuthHeaders() });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  const data = (await response.json()) as BirthdaysResponse;
  return data.birthdays ?? [];
}

// Archive a contact (deletes all reminders)
export async function archiveContact(id: string | number): Promise<ContactResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${id}/archive`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Unarchive a contact
export async function unarchiveContact(id: string | number): Promise<ContactResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${id}/unarchive`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}
