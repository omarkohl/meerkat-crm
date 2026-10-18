/**
 * Central type definitions for the Meerkat CRM application
 *
 * This file consolidates all TypeScript interfaces and types used across the application.
 * Types are re-exported from their source modules for backwards compatibility.
 */

import type { components } from './generated/api';

// Re-export types from API modules
export type { Contact, ContactsResponse, GetContactsParams, Birthday } from '../api/contacts';

export type { Note, NotesResponse } from '../api/notes';

export type { Activity, ActivityContact, ActivitiesResponse, GetActivitiesParams } from '../api/activities';

export type {
  Reminder,
  ReminderFormData,
  RemindersResponse,
  ReminderCompletion,
  CompletionsResponse,
} from '../api/reminders';

// Entities that other type modules need (kept in their own file so types/api.ts and
// types/forms.ts need not import this index, which re-exports them)
export type { User } from './entities';
import type { User } from './entities';

// Additional shared types

/**
 * Error object returned from API calls
 */
export interface ApiError {
  message: string;
  statusCode?: number;
  details?: unknown;
}

/**
 * Generic API response wrapper
 */
export interface ApiResponse<T> {
  data?: T;
  error?: ApiError;
}

/**
 * User authentication response
 */
export interface AuthResponse {
  token: string;
  user: User;
}

/**
 * Paginated list of users for admin
 */
export type UsersListResponse = Omit<components['schemas']['UsersOutputBody'], 'users'> & { users: User[] };

/**
 * Input for updating a user
 */
export type UserUpdateInput = {
  [K in keyof components['schemas']['UserUpdateBody']]?: Exclude<components['schemas']['UserUpdateBody'][K], null>;
};

/**
 * Form validation error
 */
export interface ValidationError {
  field: string;
  message: string;
}

// Reminder type is re-exported from '../api/reminders' above

/**
 * Photo metadata
 */
export interface Photo {
  id: string;
  url: string;
  thumbnailUrl?: string;
  contactId: number;
  uploadedAt: string;
}

// Re-export utility types
export type {
  FormState,
  AsyncState,
  PaginationParams,
  PaginatedResponse,
  SearchParams,
  SortParams,
  DateRange,
  DialogState,
  TableColumn,
  ActionButton,
  Notification,
  SelectOption,
  ChangeHandler,
  ClickHandler,
  SubmitHandler,
  SelectChangeHandler,
  Nullable,
  Optional,
  Maybe,
} from './utils';

// Re-export form types
export type {
  ContactFormData,
  ContactUpdateFormData,
  ActivityFormData,
  ActivityUpdateFormData,
  NoteFormData,
  NoteUpdateFormData,
  // ReminderFormData is re-exported from api/reminders above
  LoginFormData,
  RegisterFormData,
  PasswordResetRequestFormData,
  PasswordResetFormData,
  SearchFormData,
  ContactFilterFormData,
  ActivityFilterFormData,
  NoteFilterFormData,
  FieldError,
  ValidationResult,
  FieldValidator,
  FormField,
  FormConfig,
  FileUploadData,
  BulkOperationFormData,
  ImportFormData,
  ExportFormData,
  SettingsFormData,
  FormSubmissionResult,
} from './forms';

// Re-export API types
export type {
  ApiErrorResponse,
  ApiSuccessResponse,
  LoginResponse,
  RegisterResponse,
  GetContactsResponse,
  GetContactResponse,
  CreateContactResponse,
  UpdateContactResponse,
  DeleteContactResponse,
  UploadPhotoResponse,
  HealthCheckResponse,
  SearchResponse,
  GetCirclesResponse,
  StatisticsResponse,
  ExportResponse,
  ImportResponse,
  BatchOperationResponse,
  HttpMethod,
  ApiRequestConfig,
  TypedFetchOptions,
} from './api';
