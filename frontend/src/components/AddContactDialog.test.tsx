import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import i18n from '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import AddContactDialog from './AddContactDialog';
import * as contactsApi from '../api/contacts';
import * as remindersApi from '../api/reminders';

vi.mock('../api/contacts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/contacts')>()),
  createContact: vi.fn(),
}));
vi.mock('../api/reminders', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/reminders')>()),
  createReminder: vi.fn(),
}));

const created = { ID: 42, firstname: 'Ada', lastname: 'Lovelace' } as contactsApi.Contact;

const renderDialog = () => {
  const onClose = vi.fn();
  const onContactAdded = vi.fn();
  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <AddContactDialog open onClose={onClose} onContactAdded={onContactAdded} availableCircles={['Family']} />
      </DateFormatProvider>
    </SnackbarProvider>,
  );
  return { onClose, onContactAdded };
};

const type = (label: string, value: string) => {
  fireEvent.change(screen.getByRole('textbox', { name: new RegExp(`^${label}`) }), { target: { value } });
};

const create = () => fireEvent.click(screen.getByRole('button', { name: 'Create' }));

const sentContact = () => vi.mocked(contactsApi.createContact).mock.calls[0]?.[0];

describe('AddContactDialog', { timeout: 20_000 }, () => {
  beforeEach(async () => {
    vi.resetAllMocks();
    window.localStorage.setItem('dateFormat', 'iso');
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-03-10T10:00:00Z'));
    await i18n.changeLanguage('en');
    vi.mocked(contactsApi.createContact).mockResolvedValue(created);
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  test('refuses to create a contact without a first name @004-US3', () => {
    const { onContactAdded } = renderDialog();

    create();

    expect(screen.getByText('First name and last name are required')).toBeTruthy();
    expect(contactsApi.createContact).not.toHaveBeenCalled();
    expect(onContactAdded).not.toHaveBeenCalled();
  });

  test('creates the contact, drops empty rows and reports success @004-US3', async () => {
    const { onContactAdded, onClose } = renderDialog();
    type('First Name', 'Ada');
    type('Last Name', 'Lovelace');
    const addEmail = screen.getAllByRole('button', { name: 'Add' })[0] as HTMLElement;
    fireEvent.click(addEmail);
    fireEvent.click(addEmail);
    fireEvent.change(screen.getAllByRole('textbox', { name: 'Email' })[0] as HTMLElement, {
      target: { value: 'ada@example.com' },
    });

    create();

    await waitFor(() => {
      expect(onContactAdded).toHaveBeenCalledWith(42);
    });
    expect(sentContact()).toMatchObject({
      firstname: 'Ada',
      lastname: 'Lovelace',
      email: 'ada@example.com',
      emails: [{ type: 'home', value: 'ada@example.com' }],
    });
    expect(await screen.findByText('Contact created successfully!')).toBeTruthy();
    expect(onClose).toHaveBeenCalled();
  });

  test('refuses a birthday that does not match the date format @004-US3', () => {
    renderDialog();
    type('First Name', 'Ada');
    type('Birthday', 'soon');

    create();

    expect(screen.getByText('Invalid date format. Check your date format setting.')).toBeTruthy();
    expect(contactsApi.createContact).not.toHaveBeenCalled();
  });

  test('offers a birthday reminder only once a birthday is typed and creates it for the next birthday @004-US3', async () => {
    renderDialog();
    type('First Name', 'Ada');
    expect(screen.queryByLabelText('Create birthday reminder')).toBeNull();
    type('Birthday', '1990-12-10');
    fireEvent.click(screen.getByLabelText('Create birthday reminder'));

    create();

    await waitFor(() => {
      expect(remindersApi.createReminder).toHaveBeenCalled();
    });
    expect(sentContact()).toMatchObject({ birthday: '1990-12-10' });
    const [contactId, reminder] = vi.mocked(remindersApi.createReminder).mock.calls[0] ?? [];
    expect(contactId).toBe(42);
    expect(reminder).toMatchObject({ recurrence: 'yearly', by_mail: true, message: 'Birthday of Ada Lovelace' });
    const due = new Date(reminder?.remind_at ?? '');
    expect([due.getFullYear(), due.getMonth(), due.getDate()]).toEqual([2026, 11, 10]);
  });

  test('a birthday that has passed this year gets its reminder next year @004-US3', async () => {
    renderDialog();
    type('First Name', 'Ada');
    type('Birthday', '1990-01-05');
    fireEvent.click(screen.getByLabelText('Create birthday reminder'));

    create();

    await waitFor(() => {
      expect(remindersApi.createReminder).toHaveBeenCalled();
    });
    const due = new Date(vi.mocked(remindersApi.createReminder).mock.calls[0]?.[1].remind_at ?? '');
    expect([due.getFullYear(), due.getMonth(), due.getDate()]).toEqual([2027, 0, 5]);
  });

  test('collects circles from the list and from a new name, without duplicates @004-US3', async () => {
    renderDialog();
    type('First Name', 'Ada');
    fireEvent.mouseDown(screen.getByRole('combobox', { name: /Circles|Select existing/ }));
    fireEvent.click(screen.getByRole('option', { name: 'Family' }));
    for (let i = 0; i < 2; i++) {
      type('Or create new', 'Chess club');
      fireEvent.click(screen.getAllByRole('button', { name: 'Add' }).at(-1) as HTMLElement);
    }

    expect(screen.getAllByText('Chess club')).toHaveLength(1);
    create();

    await waitFor(() => {
      expect(contactsApi.createContact).toHaveBeenCalled();
    });
    expect(sentContact()?.circles).toEqual(['Family', 'Chess club']);
  });

  test('keeps the dialog open and shows the error when the server refuses @004-US3', async () => {
    vi.mocked(contactsApi.createContact).mockRejectedValue(new Error('Email already used'));
    const { onContactAdded, onClose } = renderDialog();
    type('First Name', 'Ada');

    create();

    expect((await screen.findAllByText('Email already used')).length).toBeGreaterThan(0);
    expect(onContactAdded).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });
});
