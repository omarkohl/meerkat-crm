import { test, expect } from './fixtures';
import { API_BASE_URL } from './global-setup';

// The seeded user is the first account and therefore an admin. Each test removes the
// backups it created, by name, so parallel specs are not affected.
test.describe('Backups (admin)', () => {
  test('create, download and delete a backup @011-US1', async ({ page }) => {
    await page.goto('/backups');
    await expect(page.getByRole('heading', { name: 'Backups', level: 1 })).toBeVisible();
    await expect(page.getByText(/data of ALL users/)).toBeVisible();

    const before = await (await page.request.get(`${API_BASE_URL}/admin/backups`)).json();
    const known = new Set<string>((before.backups || []).map((b: { name: string }) => b.name));

    await page.getByRole('button', { name: 'Create backup now' }).click();
    await expect(page.getByText('Backup created')).toBeVisible();

    const after = await (await page.request.get(`${API_BASE_URL}/admin/backups`)).json();
    const created = (after.backups as { name: string }[]).find((b) => !known.has(b.name));
    expect(created, 'a new backup is listed').toBeTruthy();
    const name = created!.name;

    try {
      await expect(page.getByText(name)).toBeVisible();

      const downloadPromise = page.waitForEvent('download');
      await page.getByRole('button', { name: `Download ${name}` }).click();
      const download = await downloadPromise;
      expect(download.suggestedFilename()).toBe(name);

      await page.getByRole('button', { name: `Delete ${name}` }).click();
      await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
      await expect(page.getByText('Backup deleted')).toBeVisible();
      await expect(page.getByRole('button', { name: `Download ${name}` })).toBeHidden();
      expect((await page.request.get(`${API_BASE_URL}/admin/backups/${name}`)).status()).toBe(404);
    } finally {
      await page.request.delete(`${API_BASE_URL}/admin/backups/${name}`).catch(() => {});
    }
  });

  test('the Backups page is reachable from the settings menu for admins @011-US1', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Settings' }).first().click();
    await page.getByRole('link', { name: 'Backups' }).click();
    await expect(page).toHaveURL(/\/backups$/);
  });
});
