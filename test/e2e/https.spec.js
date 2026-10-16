'use strict';

// The suite runs over HTTPS through tlsproxy (see servers.js), so these are
// the checks that only mean something on a real https:// origin. They are
// skipped in the plain-HTTP debugging mode (GONE_E2E_PLAIN=1).

const { test, expect } = require('./fixtures');
const { plain } = require('./servers');

test.skip(plain, 'needs the HTTPS harness');

test('pages are served over HTTPS in a secure context, and sending is enabled', async ({ page }) => {
  await page.goto('/');
  expect(page.url()).toMatch(/^https:\/\/127\.0\.0\.1:/);
  expect(await page.evaluate(() => window.isSecureContext)).toBe(true);
  expect(await page.evaluate(() => window.location.protocol)).toBe('https:');
  // The insecure-page banner appears only without WebCrypto; the form is live.
  await expect(page.locator('[data-i18n="compose.insecure"]')).toBeHidden();
  await page.locator('#secret').fill('x');
  await expect(page.locator('#create-secret button[type=submit]')).not.toHaveAttribute('aria-disabled', 'true');
});

test('the language cookie is set with the Secure attribute', async ({ page, context }) => {
  await page.goto('/');
  await page.locator('#lang-button').click();
  await page.locator('#lang-menu button.lang-option[data-lang="fr"]').click();
  await expect(page.locator('html')).toHaveAttribute('lang', 'fr');
  const cookie = (await context.cookies()).find((c) => c.name === 'gone_lang');
  expect(cookie, 'gone_lang cookie').toBeTruthy();
  expect(cookie.secure).toBe(true);
  expect(cookie.sameSite).toBe('Lax');
});

test('pages are served with the strict CSP', async ({ page }) => {
  const res = await page.goto('/');
  const csp = res.headers()['content-security-policy'];
  expect(csp).toContain("script-src 'self'");
  expect(csp).toContain("default-src 'none'");
});

test('pages ask search engines not to index them', async ({ page }) => {
  for (const path of ['/', '/secret/' + 'a'.repeat(32), '/about']) {
    const res = await page.goto(path);
    expect(res.headers()['x-robots-tag'], path).toBe('noindex');
  }
});
