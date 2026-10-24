import { apiUrl } from '../utils/thumbnail';
import { useCallback, useState } from 'react';
import type { ImageRef } from '../api/images';

export interface AttachedImage {
  key: string;
  id: number;
  previewUrl: string;
  takenAt?: string | null;
}

const fromRef = (ref: ImageRef): AttachedImage => ({
  key: `image-${String(ref.id)}`,
  id: ref.id,
  previewUrl: apiUrl(ref.thumb_url),
  takenAt: ref.taken_at,
});

export function useAttachedImages(initial: ImageRef[] = []) {
  const [items, setItems] = useState<AttachedImage[]>(() => initial.map(fromRef));

  const add = useCallback((refs: ImageRef[]) => {
    setItems((prev) => [...prev, ...refs.filter((r) => !prev.some((i) => i.id === r.id)).map(fromRef)]);
  }, []);

  const remove = useCallback((key: string) => {
    setItems((prev) => prev.filter((i) => i.key !== key));
  }, []);

  const move = useCallback((from: number, to: number) => {
    setItems((prev) => {
      if (from === to || from < 0 || to < 0 || from >= prev.length || to >= prev.length) return prev;
      const next = [...prev];
      const [moved] = next.splice(from, 1);
      if (moved) next.splice(to, 0, moved);
      return next;
    });
  }, []);

  const reset = useCallback((next: ImageRef[] = []) => {
    setItems(next.map(fromRef));
  }, []);

  return { items, add, remove, move, reset, imageIds: items.map((i) => i.id) };
}
