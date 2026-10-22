import { describe, test, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import '../../i18n/config';
import ImagePreview from './ImagePreview';
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

describe('ImagePreview', () => {
  test('shows the first image with a +N badge and opens it on click', () => {
    const onOpen = vi.fn();
    const parentClick = vi.fn();
    render(
      <div onClick={parentClick} role="presentation">
        <ImagePreview images={[image(1), image(2), image(3)]} onOpen={onOpen} />
      </div>,
    );
    const button = screen.getByRole('button', { name: 'Open image 1 of 3' });
    expect(button.querySelector('img')?.getAttribute('src')).toBe('/thumb/1');
    expect(screen.getByText('+2')).toBeTruthy();
    fireEvent.click(button);
    expect(onOpen).toHaveBeenCalledWith(0);
    expect(parentClick).not.toHaveBeenCalled();
  });

  test('an image-only item can show several previews in a row', () => {
    render(<ImagePreview images={[image(1), image(2), image(3), image(4)]} onOpen={vi.fn()} visible={3} />);
    expect(screen.getAllByRole('button')).toHaveLength(3);
    expect(screen.getByText('+1')).toBeTruthy();
  });

  test('renders nothing without images', () => {
    const { container } = render(<ImagePreview images={[]} onOpen={vi.fn()} />);
    expect(container.firstChild).toBeNull();
  });
});
