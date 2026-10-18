import { test, expect } from './fixtures';

test.describe('Activities', () => {
  test('the date filters use the date format from the settings @005-US3', async ({ page }) => {
    await page.addInitScript(() => window.localStorage.setItem('dateFormat', 'us'));
    await page.goto('/activities');

    const from = page.getByRole('textbox', { name: 'From' });
    await expect(from).toHaveAttribute('placeholder', 'MM/DD/YYYY');

    const request = page.waitForRequest((r) => r.url().includes('fromDate=2026-04-30'));
    await from.fill('04/30/2026');
    await request;
    await expect(from).toHaveValue('04/30/2026');
  });
});
