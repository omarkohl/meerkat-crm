import { test, expect } from './fixtures';
import { waitForLoading, searchContact, createTestContact, deleteTestContact } from './fixtures';
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

      await page.goto('/contacts');
      await waitForLoading(page);
      await searchContact(page, 'Thumbed');
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
