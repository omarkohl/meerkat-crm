import { test, expect } from './fixtures';
import { request } from '@playwright/test';
import { API_BASE_URL } from './global-setup';

// A second account used only to prove data isolation (a 409 on re-run is fine).
const USER_B = {
  username: 'e2e_isolation_userb',
  email: 'e2e_isolation_userb@example.com',
  password: 'IsolationPass123!',
};

test.describe('Multi-user isolation', () => {
  test("a user cannot see another user's contacts", async ({ page }) => {
    // Sanity: the seeded user (userA, via the shared storageState) can see Alice.
    const ownView = await page.request.get(
      `${API_BASE_URL}/contacts?search=${encodeURIComponent('Alice Johnson')}&limit=10`,
    );
    expect(ownView.ok()).toBeTruthy();
    const own = await ownView.json();
    expect((own.contacts || []).some((c: any) => c.firstname === 'Alice' && c.lastname === 'Johnson')).toBeTruthy();

    // userB gets a clean API context with no shared cookies.
    const ctx = await request.newContext();
    try {
      await ctx.post(`${API_BASE_URL}/register`, { data: USER_B }).catch(() => {});

      const login = await ctx.post(`${API_BASE_URL}/login`, {
        data: { identifier: USER_B.username, password: USER_B.password },
      });
      expect(login.ok(), 'userB login should succeed').toBeTruthy();

      // userB must not see any of userA's seeded contacts.
      const search = await ctx.get(`${API_BASE_URL}/contacts?search=${encodeURIComponent('Alice Johnson')}&limit=10`);
      expect(search.ok()).toBeTruthy();
      const result = await search.json();
      expect((result.contacts || []).some((c: any) => c.firstname === 'Alice' && c.lastname === 'Johnson')).toBeFalsy();
    } finally {
      await ctx.dispose();
    }
  });

  test("a calendar feed only contains its owner's birthdays", async ({ page }) => {
    // userA's contact with a birthday, created through the shared storageState.
    const created = await page.request.post(`${API_BASE_URL}/contacts`, {
      data: { firstname: 'E2E-IcsIsolationA', lastname: 'Owner', birthday: '1980-02-03' },
    });
    expect(created.ok()).toBeTruthy();
    const contactA = (await created.json()).contact;

    const ctx = await request.newContext();
    try {
      await ctx.post(`${API_BASE_URL}/register`, { data: USER_B }).catch(() => {});
      const login = await ctx.post(`${API_BASE_URL}/login`, {
        data: { identifier: USER_B.username, password: USER_B.password },
      });
      expect(login.ok(), 'userB login should succeed').toBeTruthy();

      const own = await ctx.post(`${API_BASE_URL}/contacts`, {
        data: { firstname: 'E2E-IcsIsolationB', lastname: 'Owner', birthday: '1981-04-05' },
      });
      expect(own.ok()).toBeTruthy();
      const contactB = (await own.json()).contact;

      const feedResponse = await ctx.post(`${API_BASE_URL}/ics-feed`);
      expect(feedResponse.status()).toBe(201);
      const { token } = await feedResponse.json();

      // The feed is fetched without any session: the token alone selects the user.
      const anonymous = await request.newContext();
      try {
        const feed = await anonymous.get(`${API_BASE_URL}/ics/${token}.ics`);
        expect(feed.status()).toBe(200);
        const body = await feed.text();
        expect(body).toContain('E2E-IcsIsolationB');
        expect(body).not.toContain('E2E-IcsIsolationA');
        expect(body).not.toContain('Alice');
      } finally {
        await anonymous.dispose();
      }

      await ctx.delete(`${API_BASE_URL}/ics-feed`);
      await ctx.delete(`${API_BASE_URL}/contacts/${contactB.ID}`);
    } finally {
      await ctx.dispose();
      await page.request.delete(`${API_BASE_URL}/contacts/${contactA.ID}`).catch(() => {});
    }
  });

  test("a user cannot list, restore or purge another user's trash", async ({ page }) => {
    const created = await page.request.post(`${API_BASE_URL}/contacts`, {
      data: { firstname: 'E2E-TrashIsolationA', lastname: 'Owner' },
    });
    expect(created.ok()).toBeTruthy();
    const contactA = (await created.json()).contact;
    expect((await page.request.delete(`${API_BASE_URL}/contacts/${contactA.ID}`)).ok()).toBeTruthy();

    const ctx = await request.newContext();
    try {
      await ctx.post(`${API_BASE_URL}/register`, { data: USER_B }).catch(() => {});
      const login = await ctx.post(`${API_BASE_URL}/login`, {
        data: { identifier: USER_B.username, password: USER_B.password },
      });
      expect(login.ok(), 'userB login should succeed').toBeTruthy();

      const trashB = await (await ctx.get(`${API_BASE_URL}/trash/contacts`)).json();
      expect((trashB.contacts || []).some((c: any) => c.id === contactA.ID)).toBeFalsy();
      expect((await ctx.post(`${API_BASE_URL}/trash/contacts/${contactA.ID}/restore`)).status()).toBe(404);
      expect((await ctx.delete(`${API_BASE_URL}/trash/contacts/${contactA.ID}`)).status()).toBe(404);
      // Emptying userB's own trash must not touch userA's.
      expect((await ctx.delete(`${API_BASE_URL}/trash/contacts`)).ok()).toBeTruthy();

      const trashA = await (await page.request.get(`${API_BASE_URL}/trash/contacts`)).json();
      expect((trashA.contacts || []).some((c: any) => c.id === contactA.ID)).toBeTruthy();
    } finally {
      await ctx.dispose();
      await page.request.delete(`${API_BASE_URL}/trash/contacts/${contactA.ID}`).catch(() => {});
    }
  });

  test('a normal user cannot list, create, download or delete backups', async ({ page }) => {
    // The seeded user is the first account and therefore an admin.
    const created = await page.request.post(`${API_BASE_URL}/admin/backups`);
    expect(created.status()).toBe(201);
    const { name } = await created.json();

    const ctx = await request.newContext();
    try {
      await ctx.post(`${API_BASE_URL}/register`, { data: USER_B }).catch(() => {});
      const login = await ctx.post(`${API_BASE_URL}/login`, {
        data: { identifier: USER_B.username, password: USER_B.password },
      });
      expect(login.ok(), 'userB login should succeed').toBeTruthy();

      expect((await ctx.get(`${API_BASE_URL}/admin/backups`)).status()).toBe(403);
      expect((await ctx.post(`${API_BASE_URL}/admin/backups`)).status()).toBe(403);
      expect((await ctx.get(`${API_BASE_URL}/admin/backups/${name}`)).status()).toBe(403);
      expect((await ctx.delete(`${API_BASE_URL}/admin/backups/${name}`)).status()).toBe(403);
    } finally {
      await ctx.dispose();
      await page.request.delete(`${API_BASE_URL}/admin/backups/${name}`).catch(() => {});
    }
  });
});
