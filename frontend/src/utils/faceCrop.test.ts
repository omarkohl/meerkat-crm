import { describe, it, expect } from 'vitest';
import { toServerCrop } from './faceCrop';

// @023-US3
describe('toServerCrop', () => {
  it('keeps pixels when the server did not scale the image', () => {
    expect(
      toServerCrop({ x: 10, y: 20, width: 100, height: 100 }, { width: 400, height: 300 }, { width: 400, height: 300 }),
    ).toEqual({
      x: 10,
      y: 20,
      size: 100,
    });
  });

  it('scales the crop down with the image', () => {
    expect(
      toServerCrop(
        { x: 1000, y: 500, width: 2000, height: 2000 },
        { width: 4000, height: 3000 },
        { width: 2000, height: 1500 },
      ),
    ).toEqual({
      x: 500,
      y: 250,
      size: 1000,
    });
  });

  it('keeps the square inside the image after rounding', () => {
    expect(
      toServerCrop(
        { x: 299.6, y: 0, width: 100.4, height: 100.4 },
        { width: 400, height: 300 },
        { width: 400, height: 300 },
      ),
    ).toEqual({
      x: 300,
      y: 0,
      size: 100,
    });
  });
});
