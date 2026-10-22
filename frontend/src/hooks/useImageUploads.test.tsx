import { describe, test, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useImageUploads } from './useImageUploads';
import type { ImageRef, UploadHandle } from '../api/images';

const upload = vi.hoisted(() => vi.fn());
vi.mock('../api/images', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/images')>()),
  uploadImage: upload,
}));

interface Pending {
  file: File;
  resolve: (ref: ImageRef) => void;
  reject: (err: unknown) => void;
  progress: (f: number) => void;
  aborted: boolean;
}

let pending: Pending[];
let revokeObjectURL: Mock<(url: string) => void>;
let createObjectURL: Mock<(obj: Blob | MediaSource) => string>;

const ref = (id: number): ImageRef => ({
  id,
  width: 10,
  height: 10,
  mime: 'image/webp',
  bytes: 1,
  taken_at: null,
  crop: null,
  url: `/img/${String(id)}`,
  thumb_url: `/thumb/${String(id)}`,
});

const png = (name = 'a.png') => new File(['x'], name, { type: 'image/png' });

beforeEach(() => {
  pending = [];
  let n = 0;
  createObjectURL = vi.fn((): string => `blob:${String(++n)}`);
  revokeObjectURL = vi.fn<(url: string) => void>();
  URL.createObjectURL = createObjectURL;
  URL.revokeObjectURL = revokeObjectURL;
  upload.mockImplementation((file: File, progress: (f: number) => void): UploadHandle => {
    const p = { file, progress, aborted: false } as Pending;
    const promise = new Promise<ImageRef>((resolve, reject) => {
      p.resolve = resolve;
      p.reject = reject;
    });
    pending.push(p);
    return {
      promise,
      abort: () => {
        p.aborted = true;
      },
    };
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('useImageUploads', () => {
  test('uploads at once, shows a local preview, and lists ids in the order the files were added', async () => {
    const { result } = renderHook(() => useImageUploads());
    act(() => {
      result.current.addFiles([png('1.png'), png('2.png')]);
    });
    expect(result.current.items.map((i) => i.status)).toEqual(['uploading', 'uploading']);
    expect(result.current.items[0]?.previewUrl).toBe('blob:1');
    expect(result.current.busy).toBe(true);
    expect(result.current.imageIds).toEqual([]);

    await act(async () => {
      pending[1]?.resolve(ref(22));
      pending[0]?.resolve(ref(11));
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(result.current.busy).toBe(false);
    });
    expect(result.current.imageIds).toEqual([11, 22]);
    expect(result.current.items[0]?.previewUrl).toBe('/thumb/11');
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:1');
  });

  test('runs at most three uploads at the same time', async () => {
    const { result } = renderHook(() => useImageUploads());
    act(() => {
      result.current.addFiles([png('1'), png('2'), png('3'), png('4'), png('5')]);
    });
    expect(pending).toHaveLength(3);
    await act(async () => {
      pending[0]?.resolve(ref(1));
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(pending).toHaveLength(4);
    });
  });

  test('reports progress per item', () => {
    const { result } = renderHook(() => useImageUploads());
    act(() => {
      result.current.addFiles([png()]);
    });
    act(() => {
      pending[0]?.progress(0.5);
    });
    expect(result.current.items[0]?.progress).toBe(0.5);
  });

  test('a failed upload is left out of the ids and can be retried', async () => {
    const { result } = renderHook(() => useImageUploads());
    act(() => {
      result.current.addFiles([png()]);
    });
    await act(async () => {
      pending[0]?.reject(new Error('boom'));
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(result.current.items[0]?.status).toBe('error');
    });
    expect(result.current.items[0]?.error).toBe('boom');
    expect(result.current.imageIds).toEqual([]);
    expect(result.current.busy).toBe(false);

    const key = result.current.items[0]?.key ?? '';
    act(() => {
      result.current.retry(key);
    });
    expect(result.current.items[0]?.status).toBe('uploading');
    await act(async () => {
      pending[1]?.resolve(ref(5));
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(result.current.imageIds).toEqual([5]);
    });
  });

  test('a file over the limit is refused without an upload', () => {
    const { result } = renderHook(() => useImageUploads());
    const big = png();
    Object.defineProperty(big, 'size', { value: 26 * 1024 * 1024 });
    act(() => {
      result.current.addFiles([big]);
    });
    expect(pending).toHaveLength(0);
    expect(result.current.items[0]?.status).toBe('error');
    expect(result.current.items[0]?.errorKind).toBe('tooLarge');
  });

  test('removing an item aborts a running upload and drops a finished one', () => {
    const { result } = renderHook(() => useImageUploads([ref(9)]));
    act(() => {
      result.current.addFiles([png()]);
    });
    act(() => {
      result.current.remove(result.current.items[1]?.key ?? '');
    });
    expect(pending[0]?.aborted).toBe(true);
    expect(result.current.items).toHaveLength(1);
    act(() => {
      result.current.remove(result.current.items[0]?.key ?? '');
    });
    expect(result.current.imageIds).toEqual([]);
  });

  test('starts with the images it is given, and reorders', () => {
    const { result } = renderHook(() => useImageUploads([ref(1), ref(2), ref(3)]));
    expect(result.current.imageIds).toEqual([1, 2, 3]);
    expect(result.current.items[0]?.previewUrl).toBe('/thumb/1');
    act(() => {
      result.current.move(0, 2);
    });
    expect(result.current.imageIds).toEqual([2, 3, 1]);
  });

  test('reset replaces everything and unmounting revokes previews', () => {
    const { result, unmount } = renderHook(() => useImageUploads());
    act(() => {
      result.current.addFiles([png()]);
    });
    act(() => {
      result.current.reset([ref(7)]);
    });
    expect(pending[0]?.aborted).toBe(true);
    expect(result.current.imageIds).toEqual([7]);
    act(() => {
      result.current.addFiles([png()]);
    });
    unmount();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:2');
  });

  test('HEIC files get no local preview, the browser cannot show them', () => {
    const { result } = renderHook(() => useImageUploads());
    act(() => {
      result.current.addFiles([new File(['x'], 'p.heic', { type: 'image/heic' })]);
    });
    expect(result.current.items[0]?.previewUrl).toBe('');
    expect(createObjectURL).not.toHaveBeenCalled();
  });
});
