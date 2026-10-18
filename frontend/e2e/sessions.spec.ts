import { test, expect } from './fixtures';
import { request } from '@playwright/test';
import { API_BASE_URL, TEST_USER } from './global-setup';

// The shared storageState session belongs to every parallel test, so these tests only ever
// sign out sessions they created themselves (found by their user agent) or use a throwaway user.
const PROBE_AGENT = 'e2e-session-probe';

test.describe('Sessions', () => {
  test('a session shows in the settings and can be signed out from there @003-US1', async ({ page }) => {
    const other = await request.newContext({ extraHTTPHeaders: { 'User-Agent': PROBE_AGENT } });
    try {
      const login = await other.post(`${API_BASE_URL}/login`, {
        data: { identifier: TEST_USER.username, password: TEST_USER.password },
      });
      expect(login.ok()).toBeTruthy();
      expect((await other.get(`${API_BASE_URL}/users/me`)).ok()).toBeTruthy();

      await page.goto('/settings');
      await expect(page.getByRole('heading', { name: 'Active sessions' })).toBeVisible();
      await expect(page.getByText('This device')).toBeVisible();
      await page.getByRole('button', { name: `Sign out ${PROBE_AGENT}` }).click();
      await expect(page.getByRole('button', { name: `Sign out ${PROBE_AGENT}` })).toBeHidden();

      // The revoked browser is out, ours is not
      expect((await other.get(`${API_BASE_URL}/users/me`)).status()).toBe(401);
      expect((await page.request.get(`${API_BASE_URL}/users/me`)).ok()).toBeTruthy();
    } finally {
      await other.dispose();
    }
  });

  test('sign out everywhere ends every session of a user and only theirs @003-US2', async () => {
    const user = {
      username: 'e2e_sessions_user',
      email: 'e2e_sessions_user@example.com',
      password: 'SessionsPass123!',
    };
    const a = await request.newContext();
    const b = await request.newContext();
    try {
      await a.post(`${API_BASE_URL}/register`, { data: user }).catch(() => {});
      for (const ctx of [a, b]) {
        const login = await ctx.post(`${API_BASE_URL}/login`, {
          data: { identifier: user.username, password: user.password },
        });
        expect(login.ok()).toBeTruthy();
      }
      const list = await (await a.get(`${API_BASE_URL}/sessions`)).json();
      expect(list.sessions.length).toBeGreaterThanOrEqual(2);
      expect(list.sessions.filter((s: { current: boolean }) => s.current)).toHaveLength(1);

      expect((await a.post(`${API_BASE_URL}/sessions/revoke-all`)).ok()).toBeTruthy();
      expect((await a.get(`${API_BASE_URL}/users/me`)).status()).toBe(401);
      expect((await b.get(`${API_BASE_URL}/users/me`)).status()).toBe(401);
    } finally {
      await a.dispose();
      await b.dispose();
    }
  });
});

test.describe('Audit log (admin)', () => {
  test('lists logins and can be filtered by event @003-US3', async ({ page }) => {
    await page.goto('/audit-log');
    await expect(page.getByRole('heading', { name: 'Audit log' })).toBeVisible();
    await expect(page.getByRole('row', { name: /testuser.*/ }).first()).toBeVisible();

    await page.getByRole('combobox', { name: 'Event' }).click();
    await page.getByRole('option', { name: 'Failed login' }).click();
    // Only failed logins are left (or none): no plain "Login" rows
    await expect(page.getByRole('cell', { name: 'Login', exact: true })).toHaveCount(0);
  });

  test('is refused for a normal user @003-US4', async () => {
    const ctx = await request.newContext();
    try {
      const user = { username: 'e2e_audit_plain', email: 'e2e_audit_plain@example.com', password: 'AuditPlain123!' };
      await ctx.post(`${API_BASE_URL}/register`, { data: user }).catch(() => {});
      await ctx.post(`${API_BASE_URL}/login`, { data: { identifier: user.username, password: user.password } });
      expect((await ctx.get(`${API_BASE_URL}/admin/audit-log`)).status()).toBe(403);
    } finally {
      await ctx.dispose();
    }
  });
});
