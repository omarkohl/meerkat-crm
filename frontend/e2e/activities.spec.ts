import { test, expect, createTestContact, deleteTestContact, openContact, pasteImage } from './fixtures';

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

  test('a contact can be added to several existing activities from its page @005-US5', async ({ page }) => {
    const host = await createTestContact(page.request);
    const guest = await createTestContact(page.request, { lastname: 'Guest' });
    const stamp = Date.now();
    const titles = [`E2E dinner ${stamp}`, `E2E hike ${stamp}`];

    try {
      await openContact(page, host);
      for (const title of titles) {
        await page.getByRole('button', { name: /^add activity$/i }).click();
        const addDialog = page.getByRole('dialog');
        await addDialog.getByRole('textbox', { name: 'Title', exact: true }).fill(title);
        await addDialog.getByRole('button', { name: /^save$/i }).click();
        await expect(addDialog).toBeHidden();
      }

      await openContact(page, guest);
      await page.getByRole('button', { name: /^add activity$/i }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('button', { name: 'Existing activities' }).click();
      for (const title of titles) {
        await dialog.getByRole('combobox', { name: 'Activities' }).fill(title);
        await page.getByRole('option', { name: new RegExp(title) }).click();
      }
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();

      for (const title of titles) {
        const entry = page.getByRole('listitem').filter({ hasText: title });
        await expect(entry.getByText(`${host.firstname} Temp`)).toBeVisible();
      }
    } finally {
      await deleteTestContact(page.request, host.ID);
      await deleteTestContact(page.request, guest.ID);
    }
  });

  test('an activity shows its images on the timeline of every contact @023-US2', async ({ page }) => {
    const contact = await createTestContact(page.request);
    const guest = await createTestContact(page.request, { lastname: 'Guest' });
    const title = `E2E photos ${Date.now()}`;

    try {
      await openContact(page, contact);
      await page.getByRole('button', { name: /^add activity$/i }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('textbox', { name: 'Title', exact: true }).fill(title);
      await pasteImage(page);
      await expect(dialog.getByRole('button', { name: /remove image/i })).toBeVisible();
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();
      const entry = page.getByRole('listitem').filter({ hasText: title });
      await expect(entry.getByRole('button', { name: /open image 1 of 1/i })).toBeVisible();

      await openContact(page, guest);
      await page.getByRole('button', { name: /^add activity$/i }).click();
      const pick = page.getByRole('dialog');
      await pick.getByRole('button', { name: 'Existing activities' }).click();
      await pick.getByRole('combobox', { name: 'Activities' }).fill(title);
      await page.getByRole('option', { name: new RegExp(title) }).click();
      await pick.getByRole('button', { name: /^save$/i }).click();
      await expect(pick).toBeHidden();
      await expect(entry.getByRole('button', { name: /open image 1 of 1/i })).toBeVisible();
      await entry.getByRole('button', { name: /open image 1 of 1/i }).click();
      await expect(page.getByRole('button', { name: 'Download' })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
      await deleteTestContact(page.request, guest.ID);
    }
  });

  test('an activity can span several days @005-US6', async ({ page }) => {
    await page.addInitScript(() => window.localStorage.setItem('dateFormat', 'us'));
    const contact = await createTestContact(page.request);
    const guest = await createTestContact(page.request, { lastname: 'Guest' });
    const title = `E2E trip ${Date.now()}`;

    try {
      await openContact(page, contact);
      await page.getByRole('button', { name: /^add activity$/i }).click();
      const addDialog = page.getByRole('dialog');
      await addDialog.getByRole('textbox', { name: 'Title', exact: true }).fill(title);
      await addDialog.getByRole('textbox', { name: 'Date', exact: true }).fill('04/30/2026');
      await addDialog.getByRole('textbox', { name: 'End date (optional)' }).fill('04/28/2026');
      await expect(addDialog.getByText('The end date must not be before the date')).toBeVisible();
      await addDialog.getByRole('textbox', { name: 'End date (optional)' }).fill('05/02/2026');
      await addDialog.getByRole('button', { name: /^save$/i }).click();
      await expect(addDialog).toBeHidden();

      const entry = page.getByRole('listitem').filter({ hasText: title });
      await expect(entry.getByText('04/30–05/02/2026')).toBeVisible();

      await openContact(page, guest);
      await page.getByRole('button', { name: /^add activity$/i }).click();
      const pickDialog = page.getByRole('dialog');
      await pickDialog.getByRole('button', { name: 'Existing activities' }).click();
      await pickDialog.getByRole('combobox', { name: 'Activities' }).fill(title);
      await page.getByRole('option', { name: new RegExp(title) }).click();
      await pickDialog.getByRole('button', { name: /^save$/i }).click();
      await expect(pickDialog).toBeHidden();
      await expect(entry.getByText('04/30–05/02/2026')).toBeVisible();

      await entry.getByRole('button', { name: 'Edit' }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('textbox', { name: 'End date (optional)' })).toHaveValue('05/02/2026');
      await dialog.getByRole('textbox', { name: 'End date (optional)' }).clear();
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();
      await expect(entry.getByText('04/30/2026')).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
      await deleteTestContact(page.request, guest.ID);
    }
  });
});
