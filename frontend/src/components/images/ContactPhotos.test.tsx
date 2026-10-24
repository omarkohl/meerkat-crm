import { describe, test, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import '../../i18n/config';
import ContactPhotos from './ContactPhotos';
import { DateFormatProvider } from '../../DateFormatProvider';
import type { ImageRef } from '../../api/images';

const image = (id: number): ImageRef => ({
  id,
  width: 10,
  height: 10,
  mime: 'image/webp',
  bytes: 1,
  taken_at: null,
  crop: null,
  url: `/full/${String(id)}`,
  thumb_url: `/thumb/${String(id)}`,
});

const setup = (props: { images?: ImageRef[] } = {}) => {
  const onAdd = vi.fn();
  render(
    <DateFormatProvider>
      <ContactPhotos contactId={7} images={props.images ?? []} mainId={null} onAdd={onAdd} onChanged={vi.fn()} />
    </DateFormatProvider>,
  );
  return { onAdd };
};

// @023-US3
describe('ContactPhotos', () => {
  test('says so when there are no photos, and adds one from the tab', () => {
    const { onAdd } = setup();
    expect(screen.getByText('No photos yet')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Add photo' }));
    expect(onAdd).toHaveBeenCalledWith();
  });

  test('photos dropped on the tab are all handed on', () => {
    const { onAdd } = setup();
    const files = [new File(['x'], 'a.png', { type: 'image/png' }), new File(['x'], 'b.png', { type: 'image/png' })];
    fireEvent.drop(screen.getByText('No photos yet'), { dataTransfer: { files } });
    expect(onAdd).toHaveBeenCalledWith(files);
  });

  test('opens a photo in the gallery', async () => {
    setup({ images: [image(1), image(2)] });
    fireEvent.click(screen.getByRole('button', { name: 'Open image 1 of 2' }));
    expect(await screen.findByText('1 / 2')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Next image' }));
    expect(await screen.findByText('2 / 2')).toBeTruthy();
  });
});
