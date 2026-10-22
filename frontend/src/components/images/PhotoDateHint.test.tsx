import { describe, test, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import '../../i18n/config';
import PhotoDateHint from './PhotoDateHint';

// @023-US5
describe('PhotoDateHint', () => {
  test('offers the capture date and passes it on a click only', () => {
    const onUse = vi.fn();
    render(<PhotoDateHint takenAt="2026-10-20T18:03:00Z" date="2026-10-22" touched={false} onUse={onUse} />);
    expect(onUse).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: /Use photo date/ }));
    expect(onUse).toHaveBeenCalledWith('2026-10-20');
  });

  test.each([
    ['has no capture date', null, '2026-10-22', false],
    ['was chosen by the user', '2026-10-20T18:03:00Z', '2026-10-22', true],
    ['is the date already', '2026-10-20T18:03:00Z', '2026-10-20', false],
  ])('stays hidden when the date %s', (_name, takenAt, date, touched) => {
    render(<PhotoDateHint takenAt={takenAt} date={date} touched={touched} onUse={vi.fn()} />);
    expect(screen.queryByRole('button')).toBeNull();
  });
});
