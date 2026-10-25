import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '../i18n/config';
import ContactInformation from './ContactInformation';
import { resolveEnabledFields } from '../contactFields';
import { DateFormatProvider } from '../DateFormatProvider';

const baseProps = {
  editingField: null,
  validationError: '',
  editValue: '',
  onEditStart: vi.fn(),
  onEditCancel: vi.fn(),
  onEditSave: vi.fn(),
  onEditValueChange: vi.fn(),
  enabledFields: resolveEnabledFields(['is_deceased', 'birthday']),
};

afterEach(cleanup);

function renderInfo(
  contact: Parameters<typeof ContactInformation>[0]['contact'],
  onUpdateContact = vi.fn().mockResolvedValue(undefined),
) {
  render(
    <DateFormatProvider>
      <ContactInformation {...baseProps} contact={contact} onUpdateContact={onUpdateContact} />
    </DateFormatProvider>,
  );
  return onUpdateContact;
}

function editDeceased() {
  fireEvent.click(screen.getByLabelText('Edit Deceased'));
}

test('a deceased contact shows the date of death and the age at death on both rows', () => {
  renderInfo({
    ID: 1,
    firstname: 'Jane',
    lastname: '',
    birthday: '1990-05-01',
    is_deceased: true,
    deceased_date: '2020-01-15',
  });

  expect(screen.getByText(/15\.01\.2020/)).toBeInTheDocument();
  expect(screen.getAllByText(/\(29 years old\)/)).toHaveLength(2);
});

test('a deceased contact without a date of death shows no age', () => {
  renderInfo({ ID: 1, firstname: 'Jane', lastname: '', birthday: '1990-05-01', is_deceased: true, deceased_date: '' });

  expect(screen.queryByText(/years old/)).not.toBeInTheDocument();
});

test('marking a contact deceased saves the flag', () => {
  const onUpdateContact = renderInfo({ ID: 1, firstname: 'Jane', lastname: '', is_deceased: false, deceased_date: '' });

  editDeceased();
  fireEvent.click(screen.getByRole('checkbox', { name: /deceased/i }));
  fireEvent.click(screen.getByText('Save'));

  expect(onUpdateContact).toHaveBeenCalledWith({ is_deceased: true, deceased_date: '' });
});

test('the date of death is typed in the date format and saved as ISO', () => {
  const onUpdateContact = renderInfo({
    ID: 1,
    firstname: 'Jane',
    lastname: '',
    birthday: '1990-05-01',
    is_deceased: true,
    deceased_date: '',
  });

  editDeceased();
  fireEvent.change(screen.getByRole('textbox', { name: 'Date of death' }), { target: { value: '15.01.2020' } });
  fireEvent.click(screen.getByText('Save'));

  expect(onUpdateContact).toHaveBeenCalledWith({ is_deceased: true, deceased_date: '2020-01-15' });
});

test('unchecking the flag clears the date of death', () => {
  const onUpdateContact = renderInfo({
    ID: 1,
    firstname: 'Jane',
    lastname: '',
    is_deceased: true,
    deceased_date: '2020-01-15',
  });

  editDeceased();
  fireEvent.click(screen.getByRole('checkbox', { name: /deceased/i }));
  fireEvent.click(screen.getByText('Save'));

  expect(onUpdateContact).toHaveBeenCalledWith({ is_deceased: false, deceased_date: '' });
});

test('a date of death before the birthday is flagged', () => {
  renderInfo({
    ID: 1,
    firstname: 'Jane',
    lastname: '',
    birthday: '1990-05-01',
    is_deceased: true,
    deceased_date: '1980-01-01',
  });

  editDeceased();

  expect(screen.getByText(/Must be a real, past date on or after the birthday/)).toBeInTheDocument();
});
