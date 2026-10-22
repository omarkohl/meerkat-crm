import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '../../i18n/config';
import ImageAttachmentField from './ImageAttachmentField';
import { useImageUploads } from '../../hooks/useImageUploads';
import { uploadImage, type ImageRef } from '../../api/images';

vi.mock('../../api/images', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api/images')>()),
  uploadImage: vi.fn(),
}));

const ref = (id: number): ImageRef => ({
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

const Harness = ({ initial = [] }: { initial?: ImageRef[] }) => {
  const uploads = useImageUploads(initial);
  return (
    <>
      <ImageAttachmentField uploads={uploads} />
      <output data-testid="ids">{uploads.imageIds.join(',')}</output>
    </>
  );
};

const png = new File(['x'], 'shot.png', { type: 'image/png' });

beforeEach(() => {
  URL.createObjectURL = () => 'blob:x';
  URL.revokeObjectURL = () => undefined;
  vi.mocked(uploadImage).mockReset();
});

describe('ImageAttachmentField', () => {
  test('a picked file is uploaded and shows as a tile that can be removed', async () => {
    vi.mocked(uploadImage).mockReturnValue({ promise: Promise.resolve(ref(5)), abort: vi.fn() });
    render(<Harness />);

    fireEvent.change(screen.getByLabelText('Add image'), { target: { files: [png] } });

    expect(await screen.findByRole('button', { name: 'Image 1 of 1' })).toBeTruthy();
    await waitFor(() => {
      expect(screen.getByTestId('ids').textContent).toBe('5');
    });

    fireEvent.click(screen.getByRole('button', { name: 'Remove image 1' }));
    expect(screen.queryByRole('button', { name: 'Image 1 of 1' })).toBeNull();
    expect(screen.getByTestId('ids').textContent).toBe('');
  });

  test('a pasted image is added, a pasted text is ignored', async () => {
    vi.mocked(uploadImage).mockReturnValue({ promise: Promise.resolve(ref(6)), abort: vi.fn() });
    render(<Harness />);

    const paste = (files: File[]) => {
      const event = new Event('paste', { bubbles: true, cancelable: true });
      Object.defineProperty(event, 'clipboardData', { value: { files, getData: () => '' } });
      document.dispatchEvent(event);
    };
    paste([]);
    expect(screen.queryByRole('button', { name: 'Image 1 of 1' })).toBeNull();
    paste([png]);
    expect(await screen.findByRole('button', { name: 'Image 1 of 1' })).toBeTruthy();
  });

  test('a failed upload names the problem on its tile and can be retried', async () => {
    vi.mocked(uploadImage)
      .mockReturnValueOnce({ promise: Promise.reject(new Error('Server said no')), abort: vi.fn() })
      .mockReturnValueOnce({ promise: Promise.resolve(ref(8)), abort: vi.fn() });
    render(<Harness />);

    fireEvent.change(screen.getByLabelText('Add image'), { target: { files: [png] } });
    expect((await screen.findByRole('alert')).textContent).toContain('Server said no');

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => {
      expect(screen.getByTestId('ids').textContent).toBe('8');
    });
    expect(screen.queryByRole('alert')).toBeNull();
  });

  test('a file that is not an image is refused with a message', async () => {
    render(<Harness />);
    const event = new Event('paste', { bubbles: true, cancelable: true });
    Object.defineProperty(event, 'clipboardData', {
      value: { files: [new File(['x'], 'a.txt', { type: 'text/plain' })], getData: () => '' },
    });
    document.dispatchEvent(event);
    expect((await screen.findByRole('alert')).textContent).toBe('Only image files can be added');
  });

  test('Alt+Arrow moves an image', () => {
    render(<Harness initial={[ref(1), ref(2)]} />);
    fireEvent.keyDown(screen.getByRole('button', { name: 'Image 1 of 2' }), { key: 'ArrowRight', altKey: true });
    expect(screen.getByTestId('ids').textContent).toBe('2,1');
  });
});
