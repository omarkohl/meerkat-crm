import { test, expect } from './fixtures';
import {
  waitForLoading,
  searchContact,
  openContactList,
  createTestContact,
  deleteTestContact,
  openContact,
} from './fixtures';
import { API_BASE_URL } from './global-setup';

// Authenticated via the shared storageState (see playwright.config.ts) — no
// per-test login needed.
test.describe('Contacts', () => {
  test('should display contacts page with filter controls @004-US1', async ({ page }) => {
    await page.goto('/contacts');

    await expect(page.getByRole('heading', { name: /contacts/i })).toBeVisible();
    await expect(page.getByLabel(/filter by circle/i)).toBeVisible();
    await expect(page.getByLabel(/sort by/i)).toBeVisible();
    await expect(page.getByRole('button', { name: /add/i })).toBeVisible();
  });

  test('should find seeded contacts via search @004-US2', async ({ page }) => {
    await page.goto('/contacts');
    await waitForLoading(page);

    await searchContact(page, 'Alice');
    await expect(page.getByText('Alice Johnson')).toBeVisible();

    await searchContact(page, 'Bob');
    await expect(page.getByText('Bob Smith')).toBeVisible();
  });

  test('header search text is discarded on navigation and on Escape @004-US2', async ({ page }) => {
    await page.goto('/contacts');
    await waitForLoading(page);
    const searchInput = page.getByRole('banner').getByPlaceholder(/search/i);

    await searchInput.fill('Ali');
    await page.getByRole('link', { name: 'Activities', exact: true }).first().click();
    await expect(page).toHaveURL(/\/activities/);
    await expect(searchInput).toHaveValue('');

    await searchInput.fill('Ali');
    // The first Escape closes the suggestions when they are open, the next one clears
    await searchInput.press('Escape');
    await searchInput.press('Escape');
    await expect(searchInput).toHaveValue('');
  });

  test('search ignores accents and case and matches the start of every word @004-US2', async ({ page, request }) => {
    const created = await createTestContact(request, { firstname: 'Zoë', lastname: 'Müllerová' });
    try {
      await page.goto('/contacts');
      await waitForLoading(page);
      await searchContact(page, 'ZOE mulle');
      await expect(page.getByText('Zoë Müllerová')).toBeVisible();
      await searchContact(page, 'müllerová zoë');
      await expect(page.getByText('Zoë Müllerová')).toBeVisible();
      await searchContact(page, 'zoe smith');
      await expect(page.getByText('Zoë Müllerová')).toBeHidden();
    } finally {
      await deleteTestContact(request, created.ID);
    }
  });

  test('searching sorts by best match and the sort menu offers it only then @004-US2', async ({ page }) => {
    await page.goto('/contacts');
    await waitForLoading(page);
    await expect(page.getByLabel(/sort by/i)).not.toContainText('Best match');
    await openContactList(page, 'Alice');
    await expect(page.getByText('Alice Johnson')).toBeVisible();
    await expect(page.getByLabel(/sort by/i)).toContainText('Best match');
  });

  test('should navigate to contact detail @004-US1', async ({ page }) => {
    await page.goto('/contacts');
    await waitForLoading(page);

    await searchContact(page, 'Alice');
    await page.getByText('Alice Johnson').click();

    await expect(page).toHaveURL(/\/contacts\/\d+/);
    await expect(page.getByText('alice@example.com')).toBeVisible();
  });

  test('should create a new contact @004-US3', async ({ page }) => {
    // Unique name per run so repeated runs don't accumulate duplicates.
    const firstname = `E2EFixture${Date.now()}`;
    const lastname = 'Contact';

    await page.goto('/contacts');
    await page.getByRole('button', { name: /add/i }).click();
    await expect(page.getByRole('dialog')).toBeVisible();

    await page.getByLabel(/first.*name/i).fill(firstname);
    await page.getByLabel(/last.*name/i).fill(lastname);
    await page.getByRole('button', { name: /create/i }).click();

    // Lands on the new contact's detail page.
    await expect(page).toHaveURL(/\/contacts\/\d+/);
    await expect(page.getByRole('heading', { name: `${firstname} ${lastname}` })).toBeVisible();

    // Clean up so runs stay idempotent.
    const contactId = page.url().match(/\/contacts\/(\d+)/)?.[1];
    expect(contactId, 'the new contact has an ID in its URL').toBeTruthy();
    await deleteTestContact(page.request, String(contactId));
  });

  test('should edit a contact name @004-US4', async ({ page }) => {
    const contact = await createTestContact(page.request, { lastname: 'Before' });

    try {
      await openContact(page, contact);

      // Enter profile edit mode via the (hover-revealed) pencil next to the name.
      await page.getByRole('button', { name: `Edit ${contact.firstname} Before` }).click();

      const lastName = page.getByLabel('Last Name', { exact: true });
      await lastName.fill('After');
      await page.getByRole('button', { name: 'Save', exact: true }).click();

      await expect(page.getByRole('heading', { name: `${contact.firstname} After` })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('should delete a contact from the detail page @004-US5', async ({ page }) => {
    const contact = await createTestContact(page.request, { lastname: 'ToDelete' });

    await openContact(page, contact);

    // Deletion is confirmed via a native confirm() dialog.
    page.once('dialog', (dialog) => dialog.accept());

    // The delete button only appears inside profile edit mode (entered via the
    // name pencil). The delete button itself has a title, so it's role-addressable.
    await page.getByRole('button', { name: `Edit ${contact.firstname} ToDelete` }).click();
    await page.getByRole('button', { name: /delete contact/i }).click();

    // Redirects back to the list, and the contact is gone.
    await expect(page).toHaveURL(/\/contacts$/);

    const lookup = await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
    expect(lookup.status()).toBe(404);
  });
});
