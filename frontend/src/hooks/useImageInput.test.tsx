import { describe, test, expect, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useImageInput } from './useImageInput';

function pasteEvent(files: File[], text = '') {
  const event = new Event('paste', { bubbles: true, cancelable: true });
  Object.defineProperty(event, 'clipboardData', { value: { files, getData: () => text } });
  return event;
}

const png = new File(['x'], 'shot.png', { type: 'image/png' });
const txt = new File(['x'], 'a.txt', { type: 'text/plain' });

describe('useImageInput', () => {
  test('passes pasted images on and swallows the paste', () => {
    const onFiles = vi.fn();
    renderHook(() => useImageInput({ onFiles, enabled: true }));
    const event = pasteEvent([png]);
    document.dispatchEvent(event);
    expect(onFiles).toHaveBeenCalledWith([png]);
    expect(event.defaultPrevented).toBe(true);
  });

  test('ignores a paste of text only', () => {
    const onFiles = vi.fn();
    renderHook(() => useImageInput({ onFiles, enabled: true }));
    const event = pasteEvent([], 'https://example.com/a.png');
    document.dispatchEvent(event);
    expect(onFiles).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
  });

  test('reports files that are not images', () => {
    const onFiles = vi.fn();
    const onReject = vi.fn();
    renderHook(() => useImageInput({ onFiles, onReject, enabled: true }));
    document.dispatchEvent(pasteEvent([txt, png]));
    expect(onFiles).toHaveBeenCalledWith([png]);
    expect(onReject).toHaveBeenCalledWith([txt]);
  });

  test('does nothing when disabled', () => {
    const onFiles = vi.fn();
    renderHook(() => useImageInput({ onFiles, enabled: false }));
    document.dispatchEvent(pasteEvent([png]));
    expect(onFiles).not.toHaveBeenCalled();
  });

  test('stops listening after unmount', () => {
    const onFiles = vi.fn();
    const { unmount } = renderHook(() => useImageInput({ onFiles, enabled: true }));
    unmount();
    document.dispatchEvent(pasteEvent([png]));
    expect(onFiles).not.toHaveBeenCalled();
  });

  test('handles dropped images', () => {
    const onFiles = vi.fn();
    const { result } = renderHook(() => useImageInput({ onFiles, enabled: true }));
    const preventDefault = vi.fn();
    result.current.dropProps.onDrop({
      preventDefault,
      dataTransfer: { files: [png] },
    } as never);
    expect(preventDefault).toHaveBeenCalled();
    expect(onFiles).toHaveBeenCalledWith([png]);
  });
});
