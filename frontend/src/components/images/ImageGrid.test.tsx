import { describe, test, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import '../../i18n/config';
import ImageGrid from './ImageGrid';
import type { ImageRef } from '../../api/images';

const image = (id: number): ImageRef => ({
  id,
  width: 100,
  height: 80,
  mime: 'image/webp',
  bytes: 1,
  taken_at: null,
  crop: null,
  url: `/full/${String(id)}`,
  thumb_url: `/thumb/${String(id)}`,
});

const setup = () => {
  const handlers = { onOpen: vi.fn(), onFrame: vi.fn(), onDelete: vi.fn() };
  render(<ImageGrid images={[image(1), image(2)]} mainId={1} {...handlers} />);
  return handlers;
};

// @023-US3
describe('ImageGrid', () => {
  test('marks the main image and offers to adjust its crop', () => {
    const { onFrame } = setup();
    expect(screen.getByTitle('Main photo')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Actions for image 1' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Adjust crop' }));
    expect(onFrame).toHaveBeenCalledWith(expect.objectContaining({ id: 1 }));
  });

  test('another image can be set as main or deleted', () => {
    const { onFrame, onDelete } = setup();
    fireEvent.click(screen.getByRole('button', { name: 'Actions for image 2' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Set as main' }));
    expect(onFrame).toHaveBeenCalledWith(expect.objectContaining({ id: 2 }));
    fireEvent.click(screen.getByRole('button', { name: 'Actions for image 2' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Delete' }));
    expect(onDelete).toHaveBeenCalledWith(expect.objectContaining({ id: 2 }));
  });

  test('opens an image', () => {
    const { onOpen } = setup();
    fireEvent.click(screen.getByRole('button', { name: 'Open image 2 of 2' }));
    expect(onOpen).toHaveBeenCalledWith(1);
  });
});
