import { describe, test, expect } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useAttachedImages } from './useAttachedImages';
import type { ImageRef } from '../api/images';

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

describe('useAttachedImages', () => {
  test('lists ids in the order the images were added, without duplicates', () => {
    const { result } = renderHook(() => useAttachedImages());
    act(() => {
      result.current.add([ref(5), ref(3)]);
      result.current.add([ref(5)]);
    });
    expect(result.current.imageIds).toEqual([5, 3]);
    expect(result.current.items[0]?.previewUrl).toBe('/thumb/5');
  });

  test('starts with the images it is given, and reorders', () => {
    const { result } = renderHook(() => useAttachedImages([ref(1), ref(2), ref(3)]));
    expect(result.current.imageIds).toEqual([1, 2, 3]);
    act(() => {
      result.current.move(0, 2);
    });
    expect(result.current.imageIds).toEqual([2, 3, 1]);
  });

  test('removing drops an image and reset replaces everything', () => {
    const { result } = renderHook(() => useAttachedImages([ref(1), ref(2)]));
    act(() => {
      result.current.remove('image-1');
    });
    expect(result.current.imageIds).toEqual([2]);
    act(() => {
      result.current.reset([ref(7)]);
    });
    expect(result.current.imageIds).toEqual([7]);
  });
});
