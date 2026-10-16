'use strict';

// Every page loads its scripts in the right order, under the CSP, with no
// errors, including the error states reached from a bad or stale link.

const { test, expect } = require('./fixtures');

const ID = '0123456789abcdef0123456789abcdef';
const KEY = 'A'.repeat(43);

const PAGES = [
  { path: '/', ready: '#create-secret' },
  { path: '/about', ready: 'main h1' },
  { path: '/request', ready: '#create-request' },
  { path: `/secret/${ID}#v1:${KEY}`, ready: '#open-secret' },
  { path: `/manage/${ID}#${KEY}`, ready: '#view-gone' },
  { path: `/request/${ID}`, ready: '#view-missing' },
  { path: `/reply/${ID}#v3:${KEY}.${KEY}`, ready: '#view-gone, #view-check' },
];

for (const p of PAGES) {
  test(`${p.path.split('#')[0].replace(ID, '{id}')} loads cleanly`, async ({ page }) => {
    await page.goto(p.path);
    await expect(page.locator(p.ready).first()).toBeVisible();
    // Scripts loaded and ran: the language picker is hidden until they do.
    await expect(page.locator('.lang-pick')).toBeVisible();
  });
}

test('a missing secret says gone after Open', async ({ page }) => {
  await page.goto(`/secret/${ID}#v1:${KEY}`);
  await page.locator('#open-secret').click();
  await expect(page.locator('#view-gone')).toBeVisible();
});

test('an unknown path gets the 404 page', async ({ page, errors }) => {
  errors.allow(/^\/no-such-page$/);
  const res = await page.goto('/no-such-page');
  expect(res.status()).toBe(404);
  await expect(page.locator('main h1')).toBeVisible();
});
