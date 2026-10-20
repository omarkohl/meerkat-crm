import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within, fireEvent } from '@testing-library/react';
import i18n from '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import { ApiError } from '../api/client';
import TwoFactorSettings from './TwoFactorSettings';
import * as api from '../api/twoFactor';

vi.mock('../api/twoFactor', () => ({
  getTwoFactorStatus: vi.fn(),
  setupTwoFactor: vi.fn(),
  enableTwoFactor: vi.fn(),
  disableTwoFactor: vi.fn(),
  regenerateRecoveryCodes: vi.fn(),
}));

const renderIt = () =>
  render(
    <SnackbarProvider>
      <TwoFactorSettings />
    </SnackbarProvider>,
  );

const CODES = ['AAAAA-BBBBB', 'CCCCC-DDDDD'];

describe('TwoFactorSettings', () => {
  beforeEach(async () => {
    vi.resetAllMocks();
    await i18n.changeLanguage('en');
  });

  test('enrolment: shows the QR code and key, enables with a code and shows the recovery codes once', async () => {
    vi.mocked(api.getTwoFactorStatus)
      .mockResolvedValueOnce({ enabled: false, recovery_codes_remaining: 0, available: true, server_ready: true })
      .mockResolvedValueOnce({ enabled: true, recovery_codes_remaining: 2, available: true, server_ready: true });
    vi.mocked(api.setupTwoFactor).mockResolvedValue({
      secret: 'JBSWY3DPEHPK3PXP',
      otpauth_uri: 'otpauth://totp/Meerkat%20CRM:me?secret=JBSWY3DPEHPK3PXP',
    });
    vi.mocked(api.enableTwoFactor).mockResolvedValue(CODES);
    renderIt();

    expect(await screen.findByText('Two-factor authentication is off.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));

    const dialog = await screen.findByRole('dialog');
    expect(await within(dialog).findByLabelText('Key')).toHaveValue('JBSWY3DPEHPK3PXP');
    const qr = await within(dialog).findByRole('img', { name: 'QR code for the authenticator app' });
    expect(qr.getAttribute('src')).toMatch(/^data:image\/svg\+xml/);

    fireEvent.change(within(dialog).getByLabelText(/^Current password/), { target: { value: 'secret-pass' } });
    fireEvent.change(within(dialog).getByLabelText(/^6-digit code from the app/), { target: { value: '123456' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Turn on' }));
    await waitFor(() => {
      expect(api.enableTwoFactor).toHaveBeenCalledWith('secret-pass', '123456');
    });

    expect(await screen.findByText('AAAAA-BBBBB')).toBeInTheDocument();
    expect(screen.getByText('CCCCC-DDDDD')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'I have saved the codes' }));
    await waitFor(() => expect(screen.queryByText('AAAAA-BBBBB')).not.toBeInTheDocument());
    expect(await screen.findByText('Two-factor authentication is on.')).toBeInTheDocument();
    expect(screen.getByText('Unused recovery codes: 2')).toBeInTheDocument();
  });

  test('a wrong code during enrolment keeps the dialog open with a message', async () => {
    vi.mocked(api.getTwoFactorStatus).mockResolvedValue({
      enabled: false,
      recovery_codes_remaining: 0,
      available: true,
      server_ready: true,
    });
    vi.mocked(api.setupTwoFactor).mockResolvedValue({ secret: 'ABC', otpauth_uri: 'otpauth://totp/x?secret=ABC' });
    vi.mocked(api.enableTwoFactor).mockRejectedValue(new ApiError('bad', 'INVALID_INPUT', 400, { field: 'code' }));
    renderIt();
    fireEvent.click(await screen.findByRole('button', { name: 'Turn on' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(await within(dialog).findByLabelText(/^Current password/), { target: { value: 'secret-pass' } });
    fireEvent.change(within(dialog).getByLabelText(/^6-digit code from the app/), {
      target: { value: '000000' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Turn on' }));
    expect(await within(dialog).findByText(/That code is not valid/)).toBeInTheDocument();
    expect(screen.queryByText('AAAAA-BBBBB')).not.toBeInTheDocument();
  });

  test('turning it off needs the password and a code, and maps a wrong password', async () => {
    vi.mocked(api.getTwoFactorStatus)
      .mockResolvedValueOnce({ enabled: true, recovery_codes_remaining: 10, available: true, server_ready: true })
      .mockResolvedValue({ enabled: false, recovery_codes_remaining: 0, available: true, server_ready: true });
    vi.mocked(api.disableTwoFactor)
      .mockRejectedValueOnce(new ApiError('bad', 'INVALID_INPUT', 400, { field: 'password' }))
      .mockResolvedValueOnce();
    renderIt();
    fireEvent.click(await screen.findByRole('button', { name: 'Turn off' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText(/^Current password/), { target: { value: 'secret-pass' } });
    fireEvent.change(within(dialog).getByLabelText(/^6-digit code from the app/), { target: { value: '654321' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Turn off' }));
    expect(await within(dialog).findByText('The password is incorrect.')).toBeInTheDocument();
    expect(api.disableTwoFactor).toHaveBeenCalledWith({ password: 'secret-pass', code: '654321' });

    // switch to a recovery code
    fireEvent.click(within(dialog).getByRole('button', { name: 'Use a recovery code' }));
    fireEvent.change(within(dialog).getByLabelText(/^Recovery code/), { target: { value: 'AAAAA-BBBBB' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Turn off' }));
    await waitFor(() => {
      expect(api.disableTwoFactor).toHaveBeenLastCalledWith({ password: 'secret-pass', recovery_code: 'AAAAA-BBBBB' });
    });
    expect((await screen.findAllByText('Two-factor authentication is off.')).length).toBeGreaterThan(0);
  });

  test('new recovery codes are shown after confirming', async () => {
    vi.mocked(api.getTwoFactorStatus).mockResolvedValue({
      enabled: true,
      recovery_codes_remaining: 3,
      available: true,
      server_ready: true,
    });
    vi.mocked(api.regenerateRecoveryCodes).mockResolvedValue(CODES);
    renderIt();
    fireEvent.click(await screen.findByRole('button', { name: 'New recovery codes' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText(/^Current password/), { target: { value: 'secret-pass' } });
    fireEvent.change(within(dialog).getByLabelText(/^6-digit code from the app/), { target: { value: '654321' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create new codes' }));
    expect(await screen.findByText('AAAAA-BBBBB')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'I have saved the codes' }));
    await waitFor(() => expect(screen.queryByText('AAAAA-BBBBB')).not.toBeInTheDocument());
  });

  test('is hidden for accounts without a password (single sign-on only)', async () => {
    vi.mocked(api.getTwoFactorStatus).mockResolvedValue({
      enabled: false,
      recovery_codes_remaining: 0,
      available: false,
      server_ready: true,
    });
    const { container } = renderIt();
    await waitFor(() => {
      expect(api.getTwoFactorStatus).toHaveBeenCalled();
    });
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  test('says that the administrator must set the encryption key before it can be turned on', async () => {
    vi.mocked(api.getTwoFactorStatus).mockResolvedValue({
      enabled: false,
      recovery_codes_remaining: 0,
      available: true,
      server_ready: false,
    });
    renderIt();
    expect(await screen.findByText(/administrator must set ENCRYPTION_KEY/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Turn on' })).not.toBeInTheDocument();
  });

  test('a lockout is shown in the current language, not the server text', async () => {
    vi.mocked(api.getTwoFactorStatus).mockResolvedValue({
      enabled: true,
      recovery_codes_remaining: 3,
      available: true,
      server_ready: true,
    });
    vi.mocked(api.disableTwoFactor).mockRejectedValue(new ApiError('Too many failed attempts.', 'UNKNOWN_ERROR', 429));
    renderIt();
    fireEvent.click(await screen.findByRole('button', { name: 'Turn off' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText(/^Current password/), { target: { value: 'secret-pass' } });
    fireEvent.change(within(dialog).getByLabelText(/^6-digit code from the app/), { target: { value: '654321' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Turn off' }));
    expect(await within(dialog).findByText('Too many failed attempts. Please try again later.')).toBeInTheDocument();
  });
});
