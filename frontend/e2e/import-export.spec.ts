import { test, expect, createTestContact, deleteTestContact, openContact, searchContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

async function readDownload(download: { createReadStream(): Promise<NodeJS.ReadableStream | null> }) {
  const stream = await download.createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream ?? []) chunks.push(Buffer.from(chunk as Buffer));
  return Buffer.concat(chunks).toString('utf-8');
}

test.describe('Import and export', () => {
  test('import a vCard file: review it, import it with its photo and find the contact in the list @015-US1', async ({
    page,
  }) => {
    const firstname = `E2EFixtureImport${Date.now()}`;
    const vcf = [
      'BEGIN:VCARD',
      'VERSION:3.0',
      `FN:${firstname} Imported`,
      `N:Imported;${firstname};;;`,
      'PHOTO;ENCODING=b;TYPE=PNG:iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==',
      'END:VCARD',
      '',
    ].join('\r\n');

    try {
      await page.goto('/contacts');
      await page.getByRole('button', { name: 'Import' }).click();
      const dialog = page.getByRole('dialog');
      const chooser = page.waitForEvent('filechooser');
      await dialog.getByText(/click to select/).click();
      await (await chooser).setFiles({ name: 'friends.vcf', mimeType: 'text/vcard', buffer: Buffer.from(vcf) });

      await expect(dialog.getByText(firstname)).toBeVisible();
      await expect(dialog.getByText('Valid')).toBeVisible();
      await dialog.getByRole('button', { name: 'Import' }).click();
      await expect(dialog.getByText('1 contacts created')).toBeVisible();
      await expect(dialog.getByText('Photos loaded: 1 of 1')).toBeVisible();
      await dialog.getByRole('button', { name: 'Done' }).click();

      await searchContact(page, firstname);
      await expect(page.getByRole('link', { name: `${firstname} Imported` })).toBeVisible();
    } finally {
      const found = await page.request.get(`${API_BASE_URL}/contacts?search=${encodeURIComponent(firstname)}`);
      for (const c of (await found.json()).contacts ?? []) await deleteTestContact(page.request, c.ID);
    }
  });

  test('import a CSV file: map the columns, review and import @015-US1', async ({ page }) => {
    const firstname = `E2EFixtureCsv${Date.now()}`;
    const csv = `First Name,Last Name\n${firstname},Spreadsheet\n`;

    try {
      await page.goto('/contacts');
      await page.getByRole('button', { name: 'Import' }).click();
      const dialog = page.getByRole('dialog');
      const chooser = page.waitForEvent('filechooser');
      await dialog.getByText(/click to select/).click();
      await (await chooser).setFiles({ name: 'people.csv', mimeType: 'text/csv', buffer: Buffer.from(csv) });

      await expect(dialog.getByText('Map Columns').first()).toBeVisible();
      await dialog.getByRole('button', { name: 'Continue' }).click();
      await expect(dialog.getByText(firstname)).toBeVisible();
      await dialog.getByRole('button', { name: 'Import' }).click();
      await expect(dialog.getByText('1 contacts created')).toBeVisible();
      await dialog.getByRole('button', { name: 'Done' }).click();

      await searchContact(page, firstname);
      await expect(page.getByRole('link', { name: `${firstname} Spreadsheet` })).toBeVisible();
    } finally {
      const found = await page.request.get(`${API_BASE_URL}/contacts?search=${encodeURIComponent(firstname)}`);
      for (const c of (await found.json()).contacts ?? []) await deleteTestContact(page.request, c.ID);
    }
  });

  test('export all contacts as a vCard file and as CSV with their names inside @016-US1', async ({ page }) => {
    const contact = await createTestContact(page.request, { lastname: 'Exported' });
    try {
      await page.goto('/');
      await page.getByRole('button', { name: 'Settings' }).click();
      await page.getByRole('link', { name: 'Data' }).click();

      const vcfDownload = page.waitForEvent('download');
      await page.getByRole('button', { name: 'Download VCF' }).click();
      const vcf = await readDownload(await vcfDownload);
      expect(vcf).toContain('BEGIN:VCARD');
      expect(vcf).toContain(`${contact.firstname} Exported`);
      await expect(page.getByText('Your contacts have been exported successfully.')).toBeVisible();

      const csvDownload = page.waitForEvent('download');
      await page.getByRole('button', { name: 'Download CSV' }).click();
      const csv = await readDownload(await csvDownload);
      expect(csv).toContain('=== CONTACTS ===');
      expect(csv).toContain(contact.firstname);
      await expect(page.getByText('Your data has been exported successfully.')).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('export one contact as a vCard file named after them @016-US1', async ({ page }) => {
    const contact = await createTestContact(page.request, { firstname: 'Zelda', lastname: "O'Brien" });

    try {
      await openContact(page, contact);
      const download = page.waitForEvent('download');
      await page.getByRole('button', { name: 'Export vCard' }).click();
      const file = await download;

      expect(file.suggestedFilename()).toBe('zelda-o-brien.vcf');
      expect(await readDownload(file)).toContain("FN:Zelda O'Brien");
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
