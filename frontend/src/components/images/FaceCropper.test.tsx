import { describe, test, expect, vi } from 'vitest';
import { render } from '@testing-library/react';
import { useEffect } from 'react';
import '../../i18n/config';
import FaceCropper from './FaceCropper';

vi.mock('react-easy-crop', () => ({
  default: function FakeCropper({
    onCropComplete,
    onMediaLoaded,
  }: {
    onCropComplete: (percent: object, pixels: object) => void;
    onMediaLoaded: (media: object) => void;
  }) {
    useEffect(() => {
      // The order of react-easy-crop: the area first, the size of the image after it.
      onCropComplete({}, { x: 0, y: 0, width: 50, height: 50 });
      onMediaLoaded({ naturalWidth: 100, naturalHeight: 80 });
      // eslint-disable-next-line react-hooks/exhaustive-deps -- like the real cropper, once per image
    }, []);
    return null;
  },
}));

// @023-US3
describe('FaceCropper', () => {
  test('reports the first area although it comes before the image size', () => {
    const onChange = vi.fn();
    render(<FaceCropper src="/x" onChange={onChange} />);
    expect(onChange).toHaveBeenCalledWith({ x: 0, y: 0, width: 50, height: 50 }, { width: 100, height: 80 });
  });
});
