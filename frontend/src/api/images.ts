// Image upload. XHR instead of fetch: only XHR reports upload progress.
import { API_BASE_URL, ApiError } from './client';
import type { components } from '../types/generated/api';

export type ImageRef = components['schemas']['ImageRef'];

export const MAX_IMAGE_BYTES = 25 * 1024 * 1024;

export interface UploadHandle {
  promise: Promise<ImageRef>;
  abort: () => void;
}

export function uploadImage(file: File, onProgress: (fraction: number) => void): UploadHandle {
  const xhr = new XMLHttpRequest();
  const promise = new Promise<ImageRef>((resolve, reject) => {
    xhr.open('POST', `${API_BASE_URL}/images`);
    xhr.withCredentials = true;
    xhr.responseType = 'json';
    xhr.upload.addEventListener('progress', (event) => {
      if (event.lengthComputable) onProgress(event.loaded / event.total);
    });
    xhr.addEventListener('load', () => {
      if (xhr.status === 401) {
        localStorage.removeItem('user_info');
        window.location.href = '/login';
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(xhr.response as ImageRef);
        return;
      }
      const body = xhr.response as { error?: { message: string; code: string } } | null;
      reject(new ApiError(body?.error?.message ?? xhr.statusText, body?.error?.code, xhr.status));
    });
    xhr.addEventListener('error', () => {
      reject(new Error('Network error'));
    });
    xhr.addEventListener('abort', () => {
      reject(new DOMException('Upload aborted', 'AbortError'));
    });
    const form = new FormData();
    form.append('file', file);
    xhr.send(form);
  });
  return {
    promise,
    abort: () => {
      xhr.abort();
    },
  };
}
