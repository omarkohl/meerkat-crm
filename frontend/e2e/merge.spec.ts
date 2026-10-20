import { test, expect, createTestContact, openContact, openContactList } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

async function duplicates(request: Parameters<typeof createTestContact>[0], tag: string) {
  const firstname = `${E2E_CONTACT_PREFIX}${tag}${Date.now()}`;
  const smith = await createTestContact(request, {
    firstname,
    lastname: 'Smith',
    emails: [{ type: '', value: 'smith@example.com' }],
    circles: [`${tag}Friends`],
  });
  const jones = await createTestContact(request, {
    firstname,
    lastname: 'Jones',
    how_we_met: 'Met at the conference',
    emails: [{ type: '', value: 'jones@example.com' }],
    circles: [`${tag}Work`],
  });
  return { firstname, smith, jones };
}

test.describe('Merge contacts', () => {
  test('selected contacts in the list are merged after choosing the differing values @004-US10', async ({ page }) => {
    const { firstname, jones } = await duplicates(page.request, 'List');
    const friend = await createTestContact(page.request, { lastname: 'Friend' });
    const day = new Date().toISOString();
    for (const [path, data] of [
      [`/contacts/${jones.ID}/notes`, { content: 'Jones note', date: day }],
      ['/activities', { title: 'Jones lunch', date: day, contact_ids: [jones.ID] }],
      [
        `/contacts/${jones.ID}/reminders`,
        { message: 'Call Jones', remind_at: day, recurrence: 'once', contact_id: jones.ID },
      ],
      [`/contacts/${friend.ID}/relationships`, { name: 'Jones', type: 'colleague', related_contact_id: jones.ID }],
    ] as const) {
      expect((await page.request.post(`${API_BASE_URL}${path}`, { data })).ok()).toBeTruthy();
    }
    await page.goto('/contacts');
    await openContactList(page, firstname);
    await page.getByRole('checkbox', { name: `Select ${firstname} Smith` }).check();
    await page.getByRole('checkbox', { name: `Select ${firstname} Jones` }).check();
    await page.getByRole('button', { name: /^Merge \(2\)$/ }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Merge contacts' })).toBeVisible();
    const target = dialog.getByRole('radiogroup', { name: 'Keep this contact' });
    await expect(target.getByRole('radio', { name: new RegExp(`${firstname} Smith`) })).toBeChecked();
    await dialog
      .getByRole('radiogroup', { name: 'Last Name' })
      .getByRole('radio', { name: /^Jones/ })
      .check();
    await expect(dialog.getByRole('radiogroup', { name: 'How We Met' })).toHaveCount(0);
    await dialog.getByRole('button', { name: 'Continue' }).click();

    await expect(dialog.getByText('1 contact will be deleted.')).toBeVisible();
    await expect(dialog.getByText(/1 e-mail address/)).toBeVisible();
    await expect(dialog.getByText(/This cannot be undone/)).toBeVisible();
    await dialog.getByRole('button', { name: 'Merge contacts' }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByRole('link', { name: `${firstname} Jones` })).toHaveCount(1);
    await expect(page.getByRole('link', { name: `${firstname} Smith` })).toHaveCount(0);
    await page.getByRole('link', { name: `${firstname} Jones` }).click();
    await expect(page.getByRole('heading', { name: `${firstname} Jones` })).toBeVisible();
    await expect(page.getByText('Met at the conference')).toBeVisible();
    await expect(page.getByText('ListFriends')).toBeVisible();
    await expect(page.getByText('ListWork')).toBeVisible();
    await expect(page.getByText('Jones note')).toBeVisible();
    await expect(page.getByText('Jones lunch')).toBeVisible();
    await page.getByRole('tab', { name: /reminders/i }).click();
    await expect(page.getByText('Call Jones')).toBeVisible();
    await page.getByRole('tab', { name: 'Relationships' }).click();
    await expect(page.getByRole('button', { name: `${friend.firstname} Friend` })).toBeVisible();

    await page.getByRole('link', { name: 'Trash' }).click();
    await expect(page.getByText(`${firstname} Jones`)).toHaveCount(1);
  });

  test('contacts selected in the list stay selected while the search changes @004-US10', async ({ page }) => {
    const { firstname } = await duplicates(page.request, 'Keep');
    const org = `Org${Date.now()}`;
    await createTestContact(page.request, { firstname: `${firstname}Third`, lastname: 'Hidden', organization: org });
    await page.goto('/contacts');
    const search = page.getByRole('textbox', { name: 'Search Contacts' });

    await search.fill(firstname);
    await page.getByRole('checkbox', { name: `Select ${firstname} Smith` }).check();
    await search.fill(org);
    await expect(page.getByRole('link', { name: `${firstname}Third Hidden` })).toBeVisible();
    await expect(page.getByRole('link', { name: `${firstname} Smith` })).toHaveCount(0);
    await page.getByRole('checkbox', { name: `Select ${firstname}Third Hidden` }).check();

    await expect(page.getByRole('button', { name: /^Merge \(2\)$/ })).toBeVisible();
    await search.fill(firstname);
    await expect(page.getByRole('checkbox', { name: `Select ${firstname} Smith` })).toBeChecked();
  });

  test('a contact page offers to merge with a searched contact and a chosen main contact @004-US10', async ({
    page,
  }) => {
    const { firstname } = await duplicates(page.request, 'Page');
    await openContact(page, { firstname, lastname: 'Smith' });
    await page.getByRole('button', { name: 'Merge with…' }).click();

    const picker = page.getByRole('dialog');
    await picker.getByLabel('Search contacts').fill(firstname);
    await picker.getByRole('checkbox', { name: `Select ${firstname} Jones` }).check();
    await expect(picker.getByRole('checkbox', { name: `Select ${firstname} Smith` })).toHaveCount(0);
    await picker.getByRole('button', { name: 'Continue' }).click();

    const dialog = page.getByRole('dialog');
    await dialog
      .getByRole('radiogroup', { name: 'Keep this contact' })
      .getByRole('radio', { name: new RegExp(`${firstname} Jones`) })
      .check();
    await dialog.getByRole('button', { name: 'Continue' }).click();
    await dialog.getByRole('button', { name: 'Merge contacts' }).click();

    await expect(page.getByRole('heading', { name: `${firstname} Jones` })).toBeVisible();
    await expect(page.getByText('Met at the conference')).toBeVisible();
    await expect(page.getByText('PageFriends')).toBeVisible();
  });
});
