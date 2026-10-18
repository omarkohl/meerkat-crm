import { test, expect, createTestContact, deleteTestContact } from './fixtures';
import { request } from '@playwright/test';
import { API_BASE_URL } from './global-setup';

// Authenticated via the shared storageState. This is the only spec that touches the
// seeded user's calendar feed (one feed per user), so it cannot race with others.
test.describe('Calendar feed (ICS)', () => {
  test('generate, subscribe, regenerate and revoke the feed URL', async ({ page }) => {
    await page.request.delete(`${API_BASE_URL}/ics-feed`);
    const contact = await createTestContact(page.request, { birthday: '1980-05-17' });
    // Calendar apps send no cookies, so read the feed with a clean context.
    const anonymous = await request.newContext();

    try {
      await page.goto('/api-tokens');
      await expect(page.getByRole('heading', { name: 'Calendar feed (ICS)' })).toBeVisible();

      await page.getByRole('button', { name: 'Generate feed URL' }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      const urlField = dialog.getByRole('textbox', { name: 'Feed URL' });
      const firstUrl = await urlField.inputValue();
      expect(firstUrl).toMatch(/\/api\/v1\/ics\/[A-Za-z0-9_-]{43}\.ics$/);
      await expect(dialog.getByRole('button', { name: 'Copy' })).toBeVisible();

      const feed = await anonymous.get(firstUrl);
      expect(feed.status()).toBe(200);
      expect(feed.headers()['content-type']).toContain('text/calendar');
      const body = await feed.text();
      expect(body).toContain('BEGIN:VCALENDAR');
      expect(body).toContain(contact.firstname);

      await dialog.getByRole('button', { name: /done/i }).click();
      await expect(dialog).toBeHidden();

      // The fetch above shows up as "last fetched" on the card.
      await page.reload();
      await expect(page.getByText(/Last fetched: /)).toBeVisible();
      await expect(page.getByText('Not fetched yet.')).toBeHidden();

      // Regenerate: the old URL stops working, the new one works.
      await page.getByRole('button', { name: 'Regenerate URL' }).click();
      await page.getByRole('dialog').getByRole('button', { name: 'Regenerate', exact: true }).click();
      const feedUrl = page.getByRole('dialog').getByRole('textbox', { name: 'Feed URL' });
      await expect(feedUrl).not.toHaveValue(firstUrl);
      const secondUrl = await feedUrl.inputValue();
      expect((await anonymous.get(firstUrl)).status()).toBe(404);
      expect((await anonymous.get(secondUrl)).status()).toBe(200);
      await page.getByRole('dialog').getByRole('button', { name: /done/i }).click();
      // A regenerated URL starts as not fetched (the card shows the response of the create call).
      await expect(page.getByText('Not fetched yet.')).toBeVisible();

      // Revoke
      await page.getByRole('button', { name: 'Revoke feed' }).click();
      await page.getByRole('dialog').getByRole('button', { name: 'Revoke', exact: true }).click();
      await expect(page.getByRole('button', { name: 'Generate feed URL' })).toBeVisible();
      expect((await anonymous.get(secondUrl)).status()).toBe(404);
    } finally {
      await anonymous.dispose();
      await deleteTestContact(page.request, contact.ID);
      await page.request.delete(`${API_BASE_URL}/ics-feed`);
    }
  });
});
