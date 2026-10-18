import { describe, test, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import EditableField from './EditableField';

const renderField = (props: Partial<React.ComponentProps<typeof EditableField>> = {}) => {
  const handlers = { onEditStart: vi.fn(), onEditCancel: vi.fn(), onEditSave: vi.fn(), onEditValueChange: vi.fn() };
  render(
    <EditableField
      icon={<span />}
      label="Nickname"
      field="nickname"
      value="Ace"
      isEditing={false}
      editValue=""
      validationError=""
      {...handlers}
      {...props}
    />,
  );
  return handlers;
};

describe('EditableField', () => {
  test('shows the value followed by its suffix @004-US4', () => {
    renderField({ displaySuffix: '(nick)' });
    expect(screen.getByText('Ace (nick)')).toBeTruthy();
  });

  test('shows the formatted value but starts editing with the raw one @004-US4', () => {
    const { onEditStart } = renderField({ value: '1990-12-10', formattedDisplayValue: '10.12.1990' });
    expect(screen.getByText('10.12.1990')).toBeTruthy();

    fireEvent.click(screen.getByRole('button'));

    expect(onEditStart).toHaveBeenCalledWith('nickname', '1990-12-10');
  });

  test('shows a placeholder mark for an empty value @004-US4', () => {
    renderField({ value: '' });
    expect(screen.getByText('—')).toBeTruthy();
  });

  test('while editing it shows the draft, reports typing, saves and cancels @004-US4', () => {
    const { onEditSave, onEditCancel, onEditValueChange } = renderField({ isEditing: true, editValue: 'Draft' });
    const input = screen.getByDisplayValue('Draft');
    expect(screen.queryByText('Ace')).toBeNull();

    fireEvent.change(input, { target: { value: 'Draft 2' } });
    expect(onEditValueChange).toHaveBeenCalledWith('Draft 2');
    const [save, cancel] = screen.getAllByRole('button');
    fireEvent.click(save as HTMLElement);
    fireEvent.click(cancel as HTMLElement);

    expect(onEditSave).toHaveBeenCalledWith('nickname');
    expect(onEditCancel).toHaveBeenCalled();
  });

  test('shows a validation error only while editing @004-US4', () => {
    renderField({ validationError: 'Too long' });
    expect(screen.queryByText('Too long')).toBeNull();
  });

  test('shows the validation error next to the field being edited @004-US4', () => {
    renderField({ isEditing: true, editValue: 'x', validationError: 'Too long' });
    expect(screen.getByText('Too long')).toBeTruthy();
  });
});
