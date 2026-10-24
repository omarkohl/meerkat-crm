import { apiUrl } from '../utils/thumbnail';
import { useCallback, useEffect, useRef, useState } from 'react';
import { uploadImage, MAX_IMAGE_BYTES, type ImageRef, type UploadHandle } from '../api/images';

const MAX_PARALLEL = 3;

export interface UploadItem {
  key: string;
  image?: ImageRef;
  previewUrl: string;
  status: 'uploading' | 'done' | 'error';
  progress: number;
  error?: string;
  errorKind?: 'tooLarge';
}

const isHeic = (file: File) => /hei[cf]/i.test(file.type) || /\.hei[cf]$/i.test(file.name);

// Browsers cannot show HEIC: no local preview, the server thumbnail replaces it when done.
const localPreview = (file: File) => (isHeic(file) ? '' : URL.createObjectURL(file));

const revoke = (url: string) => {
  if (url.startsWith('blob:')) URL.revokeObjectURL(url);
};

export function useImageUploads() {
  const [items, setItems] = useState<UploadItem[]>([]);
  const files = useRef(new Map<string, File>());
  const handles = useRef(new Map<string, UploadHandle>());
  const queue = useRef<string[]>([]);
  const running = useRef(0);
  const counter = useRef(0);
  const latest = useRef(items);
  latest.current = items;

  const patch = useCallback((key: string, change: Partial<UploadItem>) => {
    setItems((prev) => prev.map((i) => (i.key === key ? { ...i, ...change } : i)));
  }, []);

  const pump = useCallback(() => {
    while (running.current < MAX_PARALLEL && queue.current.length > 0) {
      const key = queue.current.shift();
      const file = key ? files.current.get(key) : undefined;
      if (!key || !file) continue;
      running.current++;
      const handle = uploadImage(file, (progress) => {
        patch(key, { progress });
      });
      handles.current.set(key, handle);
      handle.promise
        .then((image) => {
          files.current.delete(key);
          setItems((prev) =>
            prev.map((i) => {
              if (i.key !== key) return i;
              revoke(i.previewUrl);
              return { ...i, image, previewUrl: apiUrl(image.thumb_url), status: 'done', progress: 1 };
            }),
          );
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === 'AbortError') return;
          patch(key, { status: 'error', error: err instanceof Error ? err.message : String(err) });
        })
        .finally(() => {
          running.current--;
          handles.current.delete(key);
          pump();
        });
    }
  }, [patch]);

  const addFiles = useCallback(
    (added: File[]) => {
      const fresh: UploadItem[] = added.map((file) => {
        const key = `upload-${String(counter.current++)}`;
        if (file.size > MAX_IMAGE_BYTES) {
          return { key, previewUrl: '', status: 'error', progress: 0, errorKind: 'tooLarge' };
        }
        files.current.set(key, file);
        queue.current.push(key);
        return { key, previewUrl: localPreview(file), status: 'uploading', progress: 0 };
      });
      setItems((prev) => [...prev, ...fresh]);
      pump();
    },
    [pump],
  );

  const retry = useCallback(
    (key: string) => {
      if (!files.current.has(key)) return;
      patch(key, { status: 'uploading', progress: 0, error: undefined });
      queue.current.push(key);
      pump();
    },
    [patch, pump],
  );

  const forget = (key: string) => {
    handles.current.get(key)?.abort();
    queue.current = queue.current.filter((k) => k !== key);
    files.current.delete(key);
  };

  const remove = useCallback((key: string) => {
    forget(key);
    setItems((prev) => {
      const gone = prev.find((i) => i.key === key);
      if (gone) revoke(gone.previewUrl);
      return prev.filter((i) => i.key !== key);
    });
  }, []);

  const reset = useCallback(() => {
    setItems((prev) => {
      prev.forEach((i) => {
        forget(i.key);
        revoke(i.previewUrl);
      });
      return [];
    });
  }, []);

  useEffect(() => {
    const running = handles.current;
    return () => {
      running.forEach((h) => {
        h.abort();
      });
      latest.current.forEach((i) => {
        revoke(i.previewUrl);
      });
    };
  }, []);

  return {
    items,
    addFiles,
    remove,
    retry,
    reset,
    images: items.flatMap((i) => (i.image ? [i.image] : [])),
    busy: items.some((i) => i.status === 'uploading'),
    failed: items.some((i) => i.status === 'error'),
  };
}
