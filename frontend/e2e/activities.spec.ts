import { test, expect, createTestContact, deleteTestContact, openContact } from './fixtures';

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

  test('an activity can get another contact by searching for its name @005-US2', async ({ page }) => {
    const first = await createTestContact(page.request);
    const second = await createTestContact(page.request, { lastname: 'Second' });
    const title = `E2E edit activity ${Date.now()}`;

    try {
      await openContact(page, first);
      await page.getByRole('button', { name: /add activity/i }).click();
      const addDialog = page.getByRole('dialog');
      await addDialog.getByRole('textbox', { name: 'Title', exact: true }).fill(title);
      await addDialog.getByRole('button', { name: /^save$/i }).click();
      await expect(addDialog).toBeHidden();

      await page.getByRole('listitem').filter({ hasText: title }).getByRole('button', { name: 'Edit' }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('combobox', { name: 'Contacts' }).fill(second.firstname);
      await page.getByRole('option', { name: `${second.firstname} Second` }).click();
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();

      const entry = page.getByRole('listitem').filter({ hasText: title });
      await expect(entry.getByText(`${second.firstname} Second`)).toBeVisible();
      await expect(entry.getByText(`${first.firstname} Temp`)).toBeVisible();
    } finally {
      await deleteTestContact(page.request, first.ID);
      await deleteTestContact(page.request, second.ID);
    }
  });
});
