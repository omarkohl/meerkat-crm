import { test, expect, createTestContact, deleteTestContact, openContact } from './fixtures';

test.describe('Global search', () => {
  test('a word from a note shows under Notes with the word marked and opens its contact @022-US1', async ({ page }) => {
    const contact = await createTestContact(page.request, { lastname: 'Noted' });
    const word = `quokka${Date.now()}`;

    try {
      await openContact(page, contact);
      await page.getByRole('button', { name: /add note/i }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByLabel(/content/i).fill(`We saw a ${word} on the island`);
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();

      await page.goto('/');
      await page
        .getByRole('banner')
        .getByPlaceholder(/search/i)
        .fill(word);

      const results = page.getByRole('listbox');
      await expect(results.getByText('Notes (1)')).toBeVisible();
      const note = results.getByRole('option').filter({ hasText: 'We saw a' });
      await expect(note.getByText(word, { exact: true })).toHaveJSProperty('tagName', 'MARK');
      await expect(results.getByRole('option', { name: `Show all results for “${word}”` })).toBeVisible();

      await note.click();
      await expect(page.getByRole('heading', { name: `${contact.firstname} Noted` })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
