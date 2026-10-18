import { createHmac } from 'crypto';
import { test, expect, LOGGED_OUT, loginUser } from './fixtures';
import { request } from '@playwright/test';
import { API_BASE_URL } from './global-setup';

// RFC 6238 (SHA-1, 6 digits, 30 s), so the tests need no extra dependency.
function totp(secret: string, atMs = Date.now()): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  let bits = '';
  for (const ch of secret.toUpperCase().replace(/=+$/, '')) bits += alphabet.indexOf(ch).toString(2).padStart(5, '0');
  const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)));
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(atMs / 1000 / 30)));
  const h = createHmac('sha1', key).update(counter).digest();
  const byte = (i: number) => h[i] ?? 0;
  const o = byte(h.length - 1) & 0x0f;
  const n = ((byte(o) & 0x7f) << 24) | (byte(o + 1) << 16) | (byte(o + 2) << 8) | byte(o + 3);
  return String(n % 1_000_000).padStart(6, '0');
}

const unique = Date.now();

test.describe('Two-factor authentication', () => {
  test.describe('login', () => {
    test.use({ storageState: LOGGED_OUT });

    test('a user can turn it on, must then give a code, and can use a recovery code', async ({ page }) => {
      const user = {
        username: `e2e_2fa_${unique}`,
        email: `e2e_2fa_${unique}@example.com`,
        password: 'TwoFactorPass123!',
      };
      const api = await request.newContext();
      try {
        expect((await api.post(`${API_BASE_URL}/register`, { data: user })).ok()).toBeTruthy();
      } finally {
        await api.dispose();
      }
      await loginUser(page, user);

      // Enrol through the settings page
      await page.goto('/settings');
      await expect(page.getByRole('heading', { name: 'Two-factor authentication' })).toBeVisible();
      await page.getByRole('button', { name: 'Turn on' }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('img', { name: 'QR code for the authenticator app' })).toBeVisible();
      const secret = (await dialog.getByLabel('Key').inputValue()).trim();
      await dialog.getByLabel(/Current password/).fill(user.password);
      await dialog.getByLabel(/6-digit code from the app/).fill('000000');
      await dialog.getByRole('button', { name: 'Turn on' }).click();
      await expect(dialog.getByText(/That code is not valid/)).toBeVisible();
      await dialog.getByLabel(/6-digit code from the app/).fill(totp(secret));
      await dialog.getByRole('button', { name: 'Turn on' }).click();

      await expect(page.getByRole('heading', { name: 'Save your recovery codes' })).toBeVisible();
      const codes = await page
        .getByRole('list', { name: 'Save your recovery codes' })
        .getByRole('listitem')
        .allTextContents();
      expect(codes).toHaveLength(10);
      await page.getByRole('button', { name: 'I have saved the codes' }).click();
      await expect(page.getByText('Unused recovery codes: 10')).toBeVisible();

      // Log out and in again: the password alone is not enough
      await page.getByRole('button', { name: /logout/i }).click();
      await expect(page.getByRole('heading', { name: 'Login' })).toBeVisible({ timeout: 10000 });
      await page.getByLabel(/username or email/i).fill(user.username);
      await page.getByLabel(/^password/i).fill(user.password);
      await page.getByRole('button', { name: 'Login' }).click();
      await expect(page.getByRole('heading', { name: 'Two-factor authentication' })).toBeVisible();

      await page.getByLabel(/Authentication code/).fill('000000');
      await page.getByRole('button', { name: 'Verify' }).click();
      await expect(page.getByText(/That code is not valid/)).toBeVisible();

      // The enrolment used the current step; the next one is accepted (clock drift allowance)
      await page.getByLabel(/Authentication code/).fill(totp(secret, Date.now() + 30_000));
      await page.getByRole('button', { name: 'Verify' }).click();
      await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible({ timeout: 10000 });

      // A recovery code works once
      await page.getByRole('button', { name: /logout/i }).click();
      await expect(page.getByRole('heading', { name: 'Login' })).toBeVisible({ timeout: 10000 });
      const submitRecoveryCode = async () => {
        await page.getByLabel(/username or email/i).fill(user.username);
        await page.getByLabel(/^password/i).fill(user.password);
        await page.getByRole('button', { name: 'Login' }).click();
        await page.getByRole('button', { name: 'Use a recovery code' }).click();
        await page.getByLabel(/Recovery code/).fill(codes[0] ?? '');
        await page.getByRole('button', { name: 'Verify' }).click();
      };
      await submitRecoveryCode();
      await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible({ timeout: 10000 });
      await page.getByRole('button', { name: /logout/i }).click();
      await expect(page.getByRole('heading', { name: 'Login' })).toBeVisible({ timeout: 10000 });
      await submitRecoveryCode();
      await expect(page.getByText(/That code is not valid/)).toBeVisible();
    });
  });

  test('an admin can reset a user whose phone is lost', async ({ page }) => {
    const user = {
      username: `e2e_2fa_reset_${unique}`,
      email: `e2e_2fa_reset_${unique}@example.com`,
      password: 'TwoFactorPass123!',
    };
    const api = await request.newContext();
    try {
      expect((await api.post(`${API_BASE_URL}/register`, { data: user })).ok()).toBeTruthy();
      expect(
        (
          await api.post(`${API_BASE_URL}/login`, { data: { identifier: user.username, password: user.password } })
        ).ok(),
      ).toBeTruthy();
      const { secret } = await (await api.post(`${API_BASE_URL}/users/me/2fa/setup`)).json();
      const enabled = await api.post(`${API_BASE_URL}/users/me/2fa/enable`, {
        data: { password: user.password, code: totp(secret) },
      });
      expect(enabled.ok()).toBeTruthy();

      const loginAgain = await request.newContext();
      try {
        const body = await (
          await loginAgain.post(`${API_BASE_URL}/login`, {
            data: { identifier: user.username, password: user.password },
          })
        ).json();
        expect(body.totp_required).toBe(true);

        await page.goto('/users');
        const row = page.getByRole('row', { name: new RegExp(user.username) });
        await expect(row.getByText('2FA', { exact: true })).toBeVisible();
        await row.getByRole('button', { name: 'Reset two-factor' }).click();
        await page.getByRole('dialog').getByRole('button', { name: 'Reset' }).click();
        await expect(row.getByText('2FA', { exact: true })).toBeHidden();

        const after = await loginAgain.post(`${API_BASE_URL}/login`, {
          data: { identifier: user.username, password: user.password },
        });
        expect(after.ok()).toBeTruthy();
        expect((await after.json()).totp_required).toBeUndefined();
      } finally {
        await loginAgain.dispose();
      }
    } finally {
      await api.dispose();
    }
  });
});
