import { useEffect, useRef } from 'react';
import type { DragEvent } from 'react';

interface UseImageInputOptions {
  onFiles: (files: File[]) => void;
  onReject?: (files: File[]) => void;
  enabled: boolean;
}

export function useImageInput({ onFiles, onReject, enabled }: UseImageInputOptions) {
  const handlers = useRef({ onFiles, onReject });
  handlers.current = { onFiles, onReject };

  const accept = (files: File[]) => {
    const images = files.filter((f) => f.type.startsWith('image/'));
    const rejected = files.filter((f) => !f.type.startsWith('image/'));
    if (images.length > 0) handlers.current.onFiles(images);
    if (rejected.length > 0) handlers.current.onReject?.(rejected);
  };

  useEffect(() => {
    if (!enabled) return;
    const onPaste = (event: ClipboardEvent) => {
      const files = Array.from(event.clipboardData?.files ?? []);
      if (files.length === 0 || event.clipboardData?.getData('text/plain')) return;
      event.preventDefault();
      accept(files);
    };
    document.addEventListener('paste', onPaste);
    return () => {
      document.removeEventListener('paste', onPaste);
    };
  }, [enabled]);

  const dropProps = {
    onDragOver: (event: DragEvent) => {
      event.preventDefault();
    },
    onDrop: (event: DragEvent) => {
      event.preventDefault();
      accept(Array.from(event.dataTransfer.files));
    },
  };

  return { dropProps };
}
