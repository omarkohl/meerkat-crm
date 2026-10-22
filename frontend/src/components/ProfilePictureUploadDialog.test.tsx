import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ProfilePictureUploadDialog from './ProfilePictureUploadDialog';
import * as images from '../api/images';

vi.mock('../api/images', async (importOriginal) => ({
  ...(await importOriginal<typeof images>()),
  uploadImage: vi.fn(),
}));

const uploaded = {
  id: 7,
  width: 100,
  height: 80,
  url: '/api/v1/images/7/image?v=a',
  thumb_url: '/t',
} as images.ImageRef;

describe('ProfilePictureUploadDialog', () => {
  beforeEach(() => {
    vi.mocked(images.uploadImage).mockReturnValue({ promise: Promise.resolve(uploaded), abort: vi.fn() });
  });

  test('uploads the file first and saves the server copy', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const file = new File(['x'], 'photo.heic', { type: 'image/heic' });
    render(<ProfilePictureUploadDialog open onClose={vi.fn()} onSave={onSave} initialFile={file} crop={false} />);

    await waitFor(() => {
      expect(images.uploadImage).toHaveBeenCalledWith(file, expect.any(Function));
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Save' }));
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith(uploaded, null);
    });
  });
});
