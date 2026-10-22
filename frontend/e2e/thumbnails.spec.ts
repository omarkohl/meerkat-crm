import { test, expect, pasteImage } from './fixtures';
import { openContact, openContactList, createTestContact, deleteTestContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

// A 60x60 red PNG
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAADwAAAA8CAIAAAC1nk4lAAAAUUlEQVR4nO3OAQkAIBAAsc/0/QMYyxieMFiAzdn9zjwfSIdJS0sHSEtLB0hLSwdIS0sHSEtLB0hLSwdIS0sHSEtLB0hLSwdIS0sHSEtLB3yZvokke7GK49bPAAAAAElFTkSuQmCC',
  'base64',
);

test.describe('Contact thumbnails', () => {
  test('the list shows a cached thumbnail URL, not an embedded image @004-US6', async ({ page, request }) => {
    const created = await createTestContact(request, { firstname: 'Thumbed', lastname: 'Avatar' });
    try {
      const upload = await request.post(`${API_BASE_URL}/contacts/${created.ID}/profile_picture`, {
        multipart: { photo: { name: 'p.png', mimeType: 'image/png', buffer: PNG } },
      });
      expect(upload.ok()).toBeTruthy();

      const list = await (await request.get(`${API_BASE_URL}/contacts?search=Thumbed`)).json();
      const url: string = list.contacts[0].photo_thumbnail;
      expect(url).toMatch(/^\/api\/v1\/contacts\/\d+\/profile_picture\?thumbnail=true&v=/);

      const image = await request.get(url);
      expect(image.status()).toBe(200);
      expect(image.headers()['content-type']).toBe('image/jpeg');
      expect(image.headers()['cache-control']).toContain('max-age=31536000');
      const etag = image.headers()['etag'];
      expect(etag).toBeTruthy();
      expect((await request.get(url, { headers: { 'If-None-Match': etag ?? '' } })).status()).toBe(304);

      await page.goto('/');
      await openContactList(page, 'Thumbed');
      const avatar = page
        .getByRole('link', { name: /Thumbed/ })
        .first()
        .getByRole('img');
      await expect(avatar).toBeVisible();
      await expect(avatar).toHaveAttribute('src', /thumbnail=true/);
      await expect.poll(() => avatar.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBeGreaterThan(0);
    } finally {
      await deleteTestContact(request, created.ID);
    }
  });
});

test.describe('Profile picture', () => {
  test('paste an image to set the profile picture @004-US9', async ({ page, request }) => {
    const contact = await createTestContact(request, { firstname: 'Pasted', lastname: 'Avatar' });
    try {
      await openContact(page, contact);
      await page.getByRole('button', { name: 'Change profile picture' }).click();
      await expect(page.getByText('You can also paste an image')).toBeVisible();

      await page.evaluate((b64) => {
        const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
        const data = new DataTransfer();
        data.items.add(new File([bytes], 'shot.png', { type: 'image/png' }));
        document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
      }, PNG.toString('base64'));

      await expect(page.getByRole('slider', { name: 'Zoom' })).toBeVisible();
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect(page.getByRole('dialog')).toBeHidden();
      await expect(page.getByRole('img').first()).toBeVisible();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('keep several photos and choose the main one @023-US3', async ({ page, request }) => {
    const contact = await createTestContact(request, { firstname: 'Gallery', lastname: 'Person' });
    try {
      await openContact(page, contact);
      for (let i = 0; i < 2; i++) {
        await page.getByRole('button', { name: 'Add photo' }).click();
        await pasteImage(page);
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await expect(page.getByRole('dialog')).toBeHidden();
      }
      await expect(page.getByRole('button', { name: /Open image \d of 2/ })).toHaveCount(2);
      await page.reload();
      await expect(page.getByRole('button', { name: /Open image \d of 2/ })).toHaveCount(2);
      await page.getByRole('button', { name: 'Actions for image 1' }).click();
      await expect(page.getByRole('menuitem', { name: 'Adjust crop' })).toBeVisible();
      await page.keyboard.press('Escape');

      await page.getByRole('button', { name: 'Actions for image 2' }).click();
      await expect(page.getByRole('menuitem', { name: 'Adjust crop' })).toBeHidden();
      await page.getByRole('menuitem', { name: 'Set as main' }).click();
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect(page.getByRole('dialog')).toBeHidden();
      await page.getByRole('button', { name: 'Actions for image 2' }).click();
      await expect(page.getByRole('menuitem', { name: 'Adjust crop' })).toBeVisible();
      await page.keyboard.press('Escape');

      await page.getByRole('button', { name: 'Actions for image 1' }).click();
      await page.getByRole('menuitem', { name: 'Delete' }).click();
      await expect(page.getByRole('button', { name: /Open image \d of 1/ })).toHaveCount(1);
      await page.reload();
      await expect(page.getByRole('button', { name: /Open image \d of 1/ })).toHaveCount(1);
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });
});
