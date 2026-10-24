import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '../../i18n/config';
import ImageAttachmentField from './ImageAttachmentField';
import { useAttachedImages } from '../../hooks/useAttachedImages';
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
  const attached = useAttachedImages(initial);
  return (
    <>
      <ImageAttachmentField attached={attached} />
      <output data-testid="ids">{attached.imageIds.join(',')}</output>
    </>
  );
};

const png = new File(['x'], 'shot.png', { type: 'image/png' });

beforeEach(() => {
  vi.mocked(uploadImage).mockReset();
});

describe('ImageAttachmentField', () => {
  test('photos added in the dialog show as tiles that can be removed', async () => {
    vi.mocked(uploadImage)
      .mockReturnValueOnce({ promise: Promise.resolve(ref(5)), abort: vi.fn() })
      .mockReturnValueOnce({ promise: Promise.resolve(ref(6)), abort: vi.fn() });
    render(<Harness />);

    fireEvent.click(screen.getByRole('button', { name: 'Add photo' }));
    expect(screen.queryByRole('checkbox')).toBeNull();
    const input = document.querySelector<HTMLInputElement>('input[type="file"]');
    if (!input) throw new Error('no file input');
    fireEvent.change(input, { target: { files: [png, png] } });
    const save = await screen.findByRole('button', { name: 'Save' });
    await waitFor(() => {
      expect((save as HTMLButtonElement).disabled).toBe(false);
    });
    fireEvent.click(save);

    expect(await screen.findByRole('button', { name: 'Image 2 of 2' })).toBeTruthy();
    expect(screen.getByTestId('ids').textContent).toBe('5,6');

    fireEvent.click(screen.getByRole('button', { name: 'Remove image 1' }));
    expect(screen.getByRole('button', { name: 'Image 1 of 1' })).toBeTruthy();
    expect(screen.getByTestId('ids').textContent).toBe('6');
  });

  test('pasting outside the dialog adds nothing', () => {
    render(<Harness />);
    const event = new Event('paste', { bubbles: true, cancelable: true });
    Object.defineProperty(event, 'clipboardData', { value: { files: [png], getData: () => '' } });
    document.dispatchEvent(event);
    expect(uploadImage).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Image 1 of 1' })).toBeNull();
  });

  test('Alt+Arrow moves an image', () => {
    render(<Harness initial={[ref(1), ref(2)]} />);
    fireEvent.keyDown(screen.getByRole('button', { name: 'Image 1 of 2' }), { key: 'ArrowRight', altKey: true });
    expect(screen.getByTestId('ids').textContent).toBe('2,1');
  });
});
