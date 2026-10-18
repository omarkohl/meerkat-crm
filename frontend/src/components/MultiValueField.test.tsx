import { describe, test, expect, beforeEach } from 'vitest';
import { useState } from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import i18n from '../i18n/config';
import MultiValueField from './MultiValueField';
import type { ContactValue } from '../api/contacts';

const Harness = ({ initial = [] as ContactValue[] }) => {
  const [rows, setRows] = useState(initial);
  return <MultiValueField label="Phone" value={rows} onChange={setRows} valueType="tel" defaultType="home" />;
};

describe('MultiValueField', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
  });

  test('adds a row of the default type, takes a value and removes it again @004-US3', () => {
    render(<Harness />);

    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Phone' }), { target: { value: '+49 30 1234' } });

    expect(screen.getByRole('combobox')).toHaveProperty('value', 'Home');
    expect(screen.getByRole('textbox', { name: 'Phone' })).toHaveProperty('value', '+49 30 1234');
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    expect(screen.queryByRole('textbox', { name: 'Phone' })).toBeNull();
  });

  test('deleting a row keeps the values of the others in place @004-US3', () => {
    render(
      <Harness
        initial={[
          { type: 'home', value: '111' },
          { type: 'work', value: '222' },
          { type: 'cell', value: '333' },
        ]}
      />,
    );

    fireEvent.click(screen.getAllByRole('button', { name: 'Delete' })[1] as HTMLElement);

    expect(screen.getAllByRole('textbox', { name: 'Phone' }).map((i) => (i as HTMLInputElement).value)).toEqual([
      '111',
      '333',
    ]);
  });

  test('a custom type label is stored as typed @004-US3', () => {
    render(<Harness initial={[{ type: 'home', value: '111' }]} />);

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'Grandma' } });

    expect(screen.getByRole('combobox')).toHaveProperty('value', 'Grandma');
    expect(screen.getByRole('textbox', { name: 'Phone' })).toHaveProperty('value', '111');
  });
});
