import { test, expect } from './fixtures';
import { waitForLoading } from './fixtures';

// Authenticated via the shared storageState (see playwright.config.ts).
test.describe('Dashboard', () => {
  test('should display dashboard @010-US1', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible();
  });

  test('should display three dashboard sections @010-US1', async ({ page }) => {
    await page.goto('/');
    await waitForLoading(page);

    await expect(page.getByText('Upcoming Birthdays').first()).toBeVisible();
    await expect(page.getByText('Upcoming Reminders').first()).toBeVisible();
    await expect(page.getByText('Stay in Touch').first()).toBeVisible();
  });

  test('should render the Stay in Touch section @010-US1', async ({ page }) => {
    await page.goto('/');
    await waitForLoading(page);

    await expect(page.getByRole('heading', { name: 'Stay in Touch' })).toBeVisible();
  });
});

test.describe('Navigation', () => {
  test('should navigate to contacts from sidebar @010-US2', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: /contacts/i }).click();
    await expect(page).toHaveURL(/\/contacts/);
    await expect(page.getByRole('heading', { name: /contacts/i })).toBeVisible();
  });

  test('should navigate to activities from sidebar @010-US2', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: /activities/i }).click();
    await expect(page).toHaveURL(/\/activities/);
  });

  test('should navigate to settings from sidebar @010-US2', async ({ page }) => {
    await page.goto('/');
    // Settings is a collapsible submenu — expand it, then open Profile.
    await page.getByRole('button', { name: 'Settings' }).click();
    await page.getByRole('link', { name: 'Profile' }).click();
    await expect(page).toHaveURL(/\/settings/);
  });
});
