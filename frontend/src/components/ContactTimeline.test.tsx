import { describe, test, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import '../i18n/config';
import ContactTimeline from './ContactTimeline';
import { DateFormatProvider } from '../DateFormatProvider';
import type { Note } from '../api/notes';
import type { ImageRef } from '../api/images';

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

const note = (id: number, content: string): Note => ({
  ID: id,
  CreatedAt: '2026-10-20T12:00:00Z',
  UpdatedAt: '2026-10-20T12:00:00Z',
  DeletedAt: null,
  content,
  date: '2026-10-20T12:00:00Z',
  contact_id: 1,
  images: [image(id * 10 + 1), image(id * 10 + 2), image(id * 10 + 3)],
});

// @023-US1
describe('ContactTimeline', () => {
  test('a note with text shows one preview, like on the Notes page', () => {
    render(
      <MemoryRouter>
        <DateFormatProvider>
          <ContactTimeline
            timelineItems={[{ type: 'note', data: note(1, 'With text'), date: '2026-10-20T12:00:00Z' }]}
            onEditItem={() => undefined}
          />
        </DateFormatProvider>
      </MemoryRouter>,
    );
    expect(screen.getAllByRole('button', { name: /Open image/ })).toHaveLength(1);
  });

  test('a note with images only shows up to three previews', () => {
    render(
      <MemoryRouter>
        <DateFormatProvider>
          <ContactTimeline
            timelineItems={[{ type: 'note', data: note(2, ''), date: '2026-10-20T12:00:00Z' }]}
            onEditItem={() => undefined}
          />
        </DateFormatProvider>
      </MemoryRouter>,
    );
    expect(screen.getAllByRole('button', { name: /Open image/ })).toHaveLength(3);
  });
});
