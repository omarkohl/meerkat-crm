import { describe, expect, test } from 'vitest';
import { filenameFromDisposition } from './export';

// @016-US1
describe('filenameFromDisposition', () => {
  test('prefers the UTF-8 name over the ASCII fallback', () => {
    expect(
      filenameFromDisposition(
        `attachment; filename=zoe-muller.vcf; filename*=UTF-8''Zo%C3%AB%20M%C3%BCller.vcf`,
        'x.vcf',
      ),
    ).toBe('Zoë Müller.vcf');
  });

  test('uses the plain filename when there is no UTF-8 name', () => {
    expect(filenameFromDisposition('attachment; filename="a.csv"', 'x.csv')).toBe('a.csv');
  });

  test('falls back to the default for a missing header or a broken encoding', () => {
    expect(filenameFromDisposition(null, 'x.vcf')).toBe('x.vcf');
    expect(filenameFromDisposition(`attachment; filename*=UTF-8''%E0%A4%A.vcf`, 'x.vcf')).toBe('x.vcf');
  });
});
