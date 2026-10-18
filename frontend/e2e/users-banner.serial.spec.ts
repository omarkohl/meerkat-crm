import { test, expect } from './fixtures';
import { request } from '@playwright/test';
import { API_BASE_URL } from './global-setup';

// The e2e backend runs with private URLs allowed (BLOCK_PRIVATE_URLS unset). Once a second
// account exists, the seeded admin sees the warning on the Users page.
const SECOND_USER = {
  username: 'e2e_banner_user',
  email: 'e2e_banner_user@example.com',
  password: 'BannerPass123!',
};

test.describe('Private URL warning (admin)', () => {
  test('shows on the Users page of a multi-user instance and stays dismissed @012-US1', async ({ page }) => {
    const ctx = await request.newContext();
    try {
      await ctx.post(`${API_BASE_URL}/register`, { data: SECOND_USER }).catch(() => {});
    } finally {
      await ctx.dispose();
    }

    await page.goto('/users');
    const banner = page.getByRole('alert').filter({ hasText: 'BLOCK_PRIVATE_URLS' });
    await expect(banner).toBeVisible();

    await banner.getByRole('button', { name: /close/i }).click();
    await expect(banner).toBeHidden();

    await page.reload();
    await expect(page.getByRole('heading', { name: 'User Management' })).toBeVisible();
    await expect(banner).toBeHidden();
  });
});
