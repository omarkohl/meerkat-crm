import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact, openContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

// A 60x60 red PNG
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAADwAAAA8CAIAAAC1nk4lAAAAUUlEQVR4nO3OAQkAIBAAsc/0/QMYyxieMFiAzdn9zjwfSIdJS0sHSEtLB0hLSwdIS0sHSEtLB0hLSwdIS0sHSEtLB0hLSwdIS0sHSEtLB3yZvokke7GK49bPAAAAAElFTkSuQmCC',
  'base64',
);

// Notes and activities both render in the contact's Timeline tab. Each test
// uses a throwaway contact and cleans it up (which cascades the note/activity).
test.describe('Timeline', () => {
  test('should add a note to a contact @005-US1', async ({ page }) => {
    const contact = await createTestContact(page.request);
    const noteContent = `E2E note ${Date.now()}`;

    try {
      await openContact(page, contact);

      // Timeline is the default tab.
      await page.getByRole('button', { name: /add note/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      await dialog.getByLabel(/content/i).fill(noteContent);
      await dialog.getByRole('button', { name: /^save$/i }).click();

      await expect(dialog).toBeHidden();
      await expect(page.getByText(noteContent)).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('attach a pasted image to a note and open it @023-US1', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      await openContact(page, contact);
      await page.getByRole('button', { name: /add note/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      await page.evaluate((b64) => {
        const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
        const data = new DataTransfer();
        data.items.add(new File([bytes], 'shot.png', { type: 'image/png' }));
        document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
      }, PNG.toString('base64'));

      await expect(dialog.getByRole('button', { name: /remove image/i })).toBeVisible();
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();

      await page.getByRole('button', { name: /open image 1 of 1/i }).click();
      await expect(page.getByRole('button', { name: 'Download' })).toBeVisible();
      await page.keyboard.press('Escape');

      await page.getByRole('button', { name: /open image 1 of 1/i }).hover();
      await page.getByRole('button', { name: 'Edit', exact: true }).last().click();
      const edit = page.getByRole('dialog');
      await edit.getByRole('button', { name: /remove image/i }).click();
      await expect(edit.getByRole('button', { name: /^save$/i })).toBeDisabled();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('should add an activity to a contact @005-US2', async ({ page }) => {
    const contact = await createTestContact(page.request);
    const activityTitle = `E2E activity ${Date.now()}`;

    try {
      await openContact(page, contact);

      await page.getByRole('button', { name: /add activity/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      await dialog.getByRole('textbox', { name: 'Title', exact: true }).fill(activityTitle);
      await dialog.getByRole('button', { name: /^save$/i }).click();

      await expect(dialog).toBeHidden();
      await expect(page.getByText(activityTitle)).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('the add activity dialog preselects the contact, also when it is archived @005-US2', async ({ page }) => {
    // Archived contacts are missing from the dialog's contact list, so preselection must not rely on it
    const contact = await createTestContact(page.request);

    try {
      await openContact(page, contact);
      const archived = await page.request.post(`${API_BASE_URL}/contacts/${contact.ID}/archive`);
      expect(archived.ok()).toBeTruthy();
      await page.reload();

      await page.getByRole('button', { name: /add activity/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('button', { name: new RegExp(contact.firstname) })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
