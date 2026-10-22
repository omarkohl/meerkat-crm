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
  test('shows up to 3 images with a +N badge and opens one on click', () => {
    const onOpen = vi.fn();
    const parentClick = vi.fn();
    render(
      <div onClick={parentClick} role="presentation">
        <ImagePreview images={[image(1), image(2), image(3), image(4)]} onOpen={onOpen} />
      </div>,
    );
    const button = screen.getByRole('button', { name: 'Open image 1 of 4' });
    expect(button.querySelector('img')?.getAttribute('src')).toBe('/thumb/1');
    expect(screen.getByText('+1')).toBeTruthy();
    fireEvent.click(button);
    expect(onOpen).toHaveBeenCalledWith(0);
    expect(parentClick).not.toHaveBeenCalled();
  });

  test('renders nothing without images', () => {
    const { container } = render(<ImagePreview images={[]} onOpen={vi.fn()} />);
    expect(container.firstChild).toBeNull();
  });
});
