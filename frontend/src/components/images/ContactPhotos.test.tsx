import { describe, test, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import '../../i18n/config';
import ContactPhotos from './ContactPhotos';
import { DateFormatProvider } from '../../DateFormatProvider';
import type { ImageRef, RelatedImage } from '../../api/images';

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

const item = (id: number, type: 'note' | 'activity', label: string): RelatedImage => ({
  image: image(id),
  source: { type, id: id * 10, label, date: '2026-10-20T12:00:00Z' },
});

const setup = (props: { images?: ImageRef[]; items?: RelatedImage[]; total?: number } = {}) => {
  const items = props.items ?? [];
  const more = vi.fn().mockResolvedValue(undefined);
  const onAdd = vi.fn();
  render(
    <MemoryRouter>
      <DateFormatProvider>
        <ContactPhotos
          contactId={7}
          images={props.images ?? []}
          mainId={null}
          related={{ items, total: props.total ?? items.length, more }}
          onAdd={onAdd}
          onChanged={vi.fn()}
        />
      </DateFormatProvider>
    </MemoryRouter>,
  );
  return { more, onAdd };
};

// @023-US6
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

  test('lists the photos of the contact apart from those of notes and activities', () => {
    setup({ images: [image(1)], items: [item(2, 'activity', 'Trip'), item(3, 'note', '')] });
    expect(screen.getByText('Contact photos')).toBeTruthy();
    expect(screen.getByText('From notes and activities')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Trip' }).getAttribute('href')).toBe('/activities?search=Trip');
    expect(screen.getByRole('link', { name: 'Note' }).getAttribute('href')).toBe('#timeline-note-30');
  });

  test('one gallery runs from the contact photos on to the related photos', async () => {
    setup({ images: [image(1)], items: [item(2, 'activity', 'Trip')] });
    fireEvent.click(screen.getByRole('button', { name: 'Open image 1 of 1' }));
    expect(await screen.findByText('1 / 2')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Next image' }));
    expect(await screen.findByText('2 / 2')).toBeTruthy();
  });

  test('more related photos can be loaded on request', () => {
    const { more } = setup({ images: [image(1)], items: [item(2, 'activity', 'Trip')], total: 3 });
    fireEvent.click(screen.getByRole('button', { name: 'Show more' }));
    expect(more).toHaveBeenCalled();
  });
});
