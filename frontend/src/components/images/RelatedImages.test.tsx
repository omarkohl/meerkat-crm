import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import '../../i18n/config';
import RelatedImages from './RelatedImages';
import { DateFormatProvider } from '../../DateFormatProvider';
import { getRelatedImages, type RelatedImage } from '../../api/images';

vi.mock('../../api/images', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api/images')>()),
  getRelatedImages: vi.fn(),
}));

const item = (id: number, type: 'note' | 'activity', label: string): RelatedImage => ({
  image: {
    id,
    width: 10,
    height: 10,
    mime: 'image/webp',
    bytes: 1,
    taken_at: null,
    crop: null,
    url: `/full/${String(id)}`,
    thumb_url: `/thumb/${String(id)}`,
  },
  source: { type, id: id * 10, label, date: '2026-10-20T12:00:00Z' },
});

const renderIt = () =>
  render(
    <MemoryRouter>
      <DateFormatProvider>
        <RelatedImages contactId={7} version="v1" />
      </DateFormatProvider>
    </MemoryRouter>,
  );

// @023-US6
describe('RelatedImages', () => {
  beforeEach(() => {
    vi.mocked(getRelatedImages).mockReset();
  });

  test('shows nothing when notes and activities have no images', async () => {
    vi.mocked(getRelatedImages).mockResolvedValue({ images: [], total: 0, page: 1, limit: 12 });
    const { container } = renderIt();
    await waitFor(() => {
      expect(getRelatedImages).toHaveBeenCalled();
    });
    expect(container.textContent).toBe('');
  });

  test('lists images with their source and loads more on request', async () => {
    vi.mocked(getRelatedImages)
      .mockResolvedValueOnce({ images: [item(1, 'activity', 'Trip')], total: 2, page: 1, limit: 12 })
      .mockResolvedValueOnce({ images: [item(2, 'note', 'Hiking')], total: 2, page: 2, limit: 12 });
    renderIt();

    expect(await screen.findByText('From notes and activities')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Trip' }).getAttribute('href')).toBe('/activities?search=Trip');

    fireEvent.click(screen.getByRole('button', { name: 'Show more' }));
    expect(await screen.findByRole('link', { name: 'Hiking' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Hiking' }).getAttribute('href')).toBe('#timeline-note-20');
    expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull();
    expect(getRelatedImages).toHaveBeenLastCalledWith(7, 2, 12);
  });

  test('a note with images only is named as a note', async () => {
    vi.mocked(getRelatedImages).mockResolvedValue({ images: [item(3, 'note', '')], total: 1, page: 1, limit: 12 });
    renderIt();
    expect((await screen.findByRole('link', { name: 'Note' })).getAttribute('href')).toBe('#timeline-note-30');
  });
});
