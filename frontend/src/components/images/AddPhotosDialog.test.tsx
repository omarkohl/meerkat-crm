import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { useEffect } from 'react';
import '../../i18n/config';
import AddPhotosDialog from './AddPhotosDialog';
import * as images from '../../api/images';

vi.mock('../../api/images', async (importOriginal) => ({
  ...(await importOriginal<typeof images>()),
  uploadImage: vi.fn(),
}));

vi.mock('./FaceCropper', () => ({
  default: function FakeCropper({ src, onChange }: { src: string; onChange: (a: object, n: object) => void }) {
    useEffect(() => {
      onChange({ x: 0, y: 0, width: 50, height: 50 }, { width: 100, height: 80 });
      // eslint-disable-next-line react-hooks/exhaustive-deps -- like the real cropper, once per image
    }, []);
    return <div data-testid="cropper">{src}</div>;
  },
}));

const ref = (id: number) =>
  ({ id, width: 100, height: 80, url: `/api/v1/images/${String(id)}/image`, thumb_url: '/t' }) as images.ImageRef;
const png = (name: string) => new File(['x'], name, { type: 'image/png' });
const done = (id: number) => ({ promise: Promise.resolve(ref(id)), abort: vi.fn() });

describe('AddPhotosDialog', () => {
  beforeEach(() => {
    URL.createObjectURL = () => 'blob:x';
    URL.revokeObjectURL = () => undefined;
    vi.mocked(images.uploadImage).mockReset().mockReturnValueOnce(done(7)).mockReturnValueOnce(done(8));
  });

  test('uploads all files first and saves the server copies in order', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const files = [new File(['x'], 'photo.heic', { type: 'image/heic' }), png('b.png')];
    render(<AddPhotosDialog open onClose={vi.fn()} onSave={onSave} initialFiles={files} />);

    fireEvent.click(await screen.findByRole('button', { name: 'Save' }));
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith([ref(7), ref(8)], null);
    });
    expect(images.uploadImage).toHaveBeenCalledWith(files[0], expect.any(Function));
  });

  test('several picked files show as tiles that can be removed', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AddPhotosDialog open onClose={vi.fn()} onSave={onSave} />);
    const input = document.querySelector<HTMLInputElement>('input[type="file"]');
    if (!input) throw new Error('no file input');
    expect(input.multiple).toBe(true);

    fireEvent.change(input, { target: { files: [png('a.png'), png('b.png')] } });
    expect(await screen.findAllByRole('button', { name: /^Remove image/ })).toHaveLength(2);
    fireEvent.click(screen.getByRole('button', { name: 'Remove image 1' }));
    await waitFor(() => {
      expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Save' }).disabled).toBe(false);
    });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith([ref(8)], null);
    });
  });

  test('offers no main picture choice unless asked', () => {
    render(<AddPhotosDialog open onClose={vi.fn()} onSave={vi.fn()} />);
    expect(screen.queryByRole('checkbox')).toBeNull();
  });

  test('the main picture is framed on the first photo', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AddPhotosDialog open onClose={vi.fn()} onSave={onSave} mainDefault initialFiles={[png('a'), png('b')]} />);

    expect((await screen.findByTestId('cropper')).textContent).toContain('/api/v1/images/7/image');
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith([ref(7), ref(8)], { x: 0, y: 0, size: 50 });
    });
  });

  test('unchecked, the photos are saved without a crop', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AddPhotosDialog open onClose={vi.fn()} onSave={onSave} mainDefault initialFiles={[png('a')]} />);

    fireEvent.click(screen.getByRole('checkbox', { name: 'Use the first photo as main picture' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Save' }));
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith([ref(7)], null);
    });
    expect(screen.queryByTestId('cropper')).toBeNull();
  });

  test('a locked box stays checked', () => {
    render(<AddPhotosDialog open onClose={vi.fn()} onSave={vi.fn()} mainDefault mainLocked />);
    const box = screen.getByRole<HTMLInputElement>('checkbox', { name: 'Use the first photo as main picture' });
    expect(box.checked).toBe(true);
    expect(box.disabled).toBe(true);
  });

  test('a failed upload blocks saving until it is retried', async () => {
    vi.mocked(images.uploadImage)
      .mockReset()
      .mockReturnValueOnce({ promise: Promise.reject(new Error('Server said no')), abort: vi.fn() })
      .mockReturnValueOnce(done(7));
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AddPhotosDialog open onClose={vi.fn()} onSave={onSave} initialFiles={[png('a')]} />);

    expect((await screen.findByRole('alert')).textContent).toContain('Server said no');
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Save' }).disabled).toBe(true);
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => {
      expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Save' }).disabled).toBe(false);
    });
  });
});
