import { test, expect, createTestContact, deleteTestContact, openContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

// Every test uses its own uniquely named contact and never empties the whole trash,
// because the seeded user's trash is shared with the other specs running in parallel.
test.describe('Trash', () => {
  test('a deleted contact appears in the trash and can be restored @008-US1', async ({ page }) => {
    const contact = await createTestContact(page.request, { lastname: 'Restorable' });
    const fullName = `${contact.firstname} Restorable`;

    try {
      // Delete through the UI (confirmed via a native confirm() dialog).
      await openContact(page, contact);
      page.once('dialog', (dialog) => dialog.accept());
      await page.getByRole('button', { name: `Edit ${fullName}` }).click();
      await page.getByRole('button', { name: /delete contact/i }).click();
      await expect(page).toHaveURL(/\/contacts$/);
      expect((await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`)).status()).toBe(404);

      // It is in the trash, reached from the sidebar.
      await page.getByRole('link', { name: 'Trash' }).click();
      await expect(page).toHaveURL(/\/trash$/);
      await expect(page.getByRole('heading', { name: 'Trash', level: 1 })).toBeVisible();
      await expect(page.getByText(fullName)).toBeVisible();

      // Restoring removes it from the trash and makes it visible again.
      await page.getByRole('button', { name: `Restore ${fullName}` }).click();
      await expect(page.getByText(fullName)).toBeHidden();

      await openContact(page, contact);
      expect((await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`)).status()).toBe(200);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('a contact can be deleted permanently from the trash @008-US2', async ({ page }) => {
    const contact = await createTestContact(page.request, { lastname: 'Purgeable' });
    const fullName = `${contact.firstname} Purgeable`;
    await page.request.delete(`${API_BASE_URL}/contacts/${contact.ID}`);

    await page.goto('/trash');
    await expect(page.getByText(fullName)).toBeVisible();

    // Cancelling keeps it.
    await page.getByRole('button', { name: `Delete ${fullName} permanently` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByText(fullName)).toBeVisible();

    await page.getByRole('button', { name: `Delete ${fullName} permanently` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Delete permanently' }).click();
    await expect(page.getByText(fullName)).toBeHidden();

    // Gone for good: neither restorable nor purgeable again.
    const restore = await page.request.post(`${API_BASE_URL}/trash/contacts/${contact.ID}/restore`);
    expect(restore.status()).toBe(404);
    const trash = await (await page.request.get(`${API_BASE_URL}/trash/contacts`)).json();
    expect((trash.contacts || []).some((c: { id: number }) => c.id === contact.ID)).toBeFalsy();
  });

  test('a trashed contact is hidden from the contact list and search @008-US3', async ({ page }) => {
    const contact = await createTestContact(page.request, { lastname: 'Hidden' });
    try {
      await page.request.delete(`${API_BASE_URL}/contacts/${contact.ID}`);
      const search = await page.request.get(`${API_BASE_URL}/contacts?search=${encodeURIComponent(contact.firstname)}`);
      expect((await search.json()).contacts || []).toHaveLength(0);
    } finally {
      await page.request.delete(`${API_BASE_URL}/trash/contacts/${contact.ID}`).catch(() => {});
    }
  });
});
