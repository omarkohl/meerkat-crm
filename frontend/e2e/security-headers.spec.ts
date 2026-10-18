import { test, expect, LOGGED_OUT } from './fixtures';
import { API_BASE_URL } from './global-setup';

// What the browser receives from the two kinds of responses: the SPA (served by nginx in
// production, by scripts/e2e-stack.mjs here with the same headers) and the API (the real
// backend, proxied unchanged).
test.describe('Security headers', () => {
  test.use({ storageState: LOGGED_OUT });

  test('the SPA is served with an enforcing, strict Content-Security-Policy @014-US1', async ({ request }) => {
    for (const path of ['/', '/contacts', '/manifest.json']) {
      const headers = (await request.get(path)).headers();
      const csp = headers['content-security-policy'];
      expect(csp, path).toContain("default-src 'self'");
      expect(csp, path).toContain("frame-ancestors 'none'");
      expect(csp, path).toContain("object-src 'none'");
      expect(csp?.match(/script-src[^;]*/)?.[0], path).toBe("script-src 'self'");
      expect(headers['content-security-policy-report-only'], path).toBeUndefined();
      expect(headers['x-frame-options'], path).toBe('DENY');
      expect(headers['x-content-type-options'], path).toBe('nosniff');
      expect(headers['referrer-policy'], path).toBe('strict-origin-when-cross-origin');
      expect(headers['permissions-policy'], path).toContain('camera=()');
      expect(headers['cross-origin-opener-policy'], path).toBe('same-origin');
      expect(headers['cross-origin-resource-policy'], path).toBe('same-origin');
      // Plain HTTP here: no HSTS
      expect(headers['strict-transport-security'], path).toBeUndefined();
    }
  });

  test('API responses carry the backend headers once, with the strict policy @014-US1', async ({ request }) => {
    const response = await request.get(`${API_BASE_URL}/contacts`);
    expect(response.status()).toBe(401);
    const headers = response.headers();
    expect(headers['content-security-policy']).toBe("default-src 'none'; frame-ancestors 'none'");
    expect(headers['x-frame-options']).toBe('DENY');
    expect(headers['x-content-type-options']).toBe('nosniff');
    expect(headers['cross-origin-resource-policy']).toBe('same-site');
    // A header sent twice would be joined with a comma
    for (const name of ['content-security-policy', 'x-frame-options', 'referrer-policy', 'x-content-type-options']) {
      expect(headers[name], name).not.toContain(',');
    }
  });

  test('the login page runs without Content-Security-Policy violations @014-US1', async ({ page }) => {
    // The guard in fixtures.ts fails the test on any violation.
    await page.goto('/');
    await expect(page.getByRole('heading', { name: /login/i })).toBeVisible();
  });

  test('the policy is enforced and the guard sees violations @014-US1', async ({ browser, baseURL }) => {
    // A plain page (not the guarded fixture): an inline script must be blocked and reported.
    const plain = await browser.newPage({ baseURL });
    try {
      await plain.goto('/');
      const directive = await plain.evaluate(
        () =>
          new Promise<string>((resolve) => {
            document.addEventListener('securitypolicyviolation', (e) => resolve(e.violatedDirective), { once: true });
            const script = document.createElement('script');
            script.textContent = 'window.__ran = true';
            document.head.appendChild(script);
          }),
      );
      expect(directive).toContain('script-src');
      expect(await plain.evaluate(() => (window as unknown as { __ran?: boolean }).__ran)).toBeUndefined();
    } finally {
      await plain.close();
    }
  });
});
