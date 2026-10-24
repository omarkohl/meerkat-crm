import { describe, test, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { useRelatedImages } from './useRelatedImages';
import { getRelatedImages, type RelatedImage } from '../api/images';

vi.mock('../api/images', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/images')>()),
  getRelatedImages: vi.fn(),
}));

const item = (id: number): RelatedImage =>
  ({ image: { id }, source: { type: 'note', id, label: '', date: '2026-10-20T12:00:00Z' } }) as RelatedImage;

// @023-US6
describe('useRelatedImages', () => {
  beforeEach(() => {
    vi.mocked(getRelatedImages).mockReset();
  });

  test('loads the first page and appends the next one on request', async () => {
    vi.mocked(getRelatedImages)
      .mockResolvedValueOnce({ images: [item(1)], total: 2, page: 1, limit: 12 })
      .mockResolvedValueOnce({ images: [item(2)], total: 2, page: 2, limit: 12 });
    const { result } = renderHook(() => useRelatedImages(7, 'v1'));

    await waitFor(() => {
      expect(result.current.total).toBe(2);
    });
    await act(async () => {
      await result.current.more();
    });
    expect(result.current.items.map((i) => i.image.id)).toEqual([1, 2]);
    expect(getRelatedImages).toHaveBeenLastCalledWith(7, 2, 12);
  });

  test('asking for more twice in a row loads the next page once', async () => {
    vi.mocked(getRelatedImages)
      .mockResolvedValueOnce({ images: [item(1)], total: 3, page: 1, limit: 12 })
      .mockResolvedValueOnce({ images: [item(2)], total: 3, page: 2, limit: 12 });
    const { result } = renderHook(() => useRelatedImages(7, 'v1'));

    await waitFor(() => {
      expect(result.current.total).toBe(3);
    });
    await act(async () => {
      await Promise.all([result.current.more(), result.current.more()]);
    });
    expect(result.current.items.map((i) => i.image.id)).toEqual([1, 2]);
    expect(getRelatedImages).toHaveBeenCalledTimes(2);
  });

  test('a page that arrives after the notes changed is dropped', async () => {
    let late: (value: Awaited<ReturnType<typeof getRelatedImages>>) => void = () => undefined;
    vi.mocked(getRelatedImages)
      .mockResolvedValueOnce({ images: [item(1)], total: 2, page: 1, limit: 12 })
      .mockReturnValueOnce(
        new Promise((resolve) => {
          late = resolve;
        }),
      )
      .mockResolvedValueOnce({ images: [item(5)], total: 1, page: 1, limit: 12 });
    const { result, rerender } = renderHook(({ version }) => useRelatedImages(7, version), {
      initialProps: { version: 'v1' },
    });
    await waitFor(() => {
      expect(result.current.total).toBe(2);
    });

    let pending: Promise<void> = Promise.resolve();
    act(() => {
      pending = result.current.more();
    });
    rerender({ version: 'v2' });
    await waitFor(() => {
      expect(result.current.items.map((i) => i.image.id)).toEqual([5]);
    });
    await act(async () => {
      late({ images: [item(2)], total: 2, page: 2, limit: 12 });
      await pending;
    });
    expect(result.current.items.map((i) => i.image.id)).toEqual([5]);
    expect(result.current.total).toBe(1);
  });
});
