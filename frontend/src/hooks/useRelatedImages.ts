import { useCallback, useEffect, useRef, useState } from 'react';
import { getRelatedImages, type RelatedImage } from '../api/images';
import { handleError } from '../utils/errorHandler';

const PAGE = 12;

// `version` changes when the notes or activities of the contact do, so the list is read again.
export function useRelatedImages(contactId: number, version: string) {
  const [items, setItems] = useState<RelatedImage[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const loading = useRef(false);
  const generation = useRef(0);

  useEffect(() => {
    const gen = ++generation.current;
    loading.current = false;
    getRelatedImages(contactId, 1, PAGE)
      .then((first) => {
        if (gen !== generation.current) return;
        setItems(first.images);
        setTotal(first.total);
        setPage(1);
      })
      .catch((err: unknown) => {
        handleError(err, { operation: 'loading related images' });
      });
  }, [contactId, version]);

  const more = useCallback(async () => {
    if (loading.current) return;
    loading.current = true;
    const gen = generation.current;
    try {
      const next = await getRelatedImages(contactId, page + 1, PAGE);
      if (gen !== generation.current) return;
      setItems((prev) => [...prev, ...next.images]);
      setTotal(next.total);
      setPage(page + 1);
    } catch (err) {
      handleError(err, { operation: 'loading related images' });
    } finally {
      if (gen === generation.current) loading.current = false;
    }
  }, [contactId, page]);

  return { items, total, more };
}
