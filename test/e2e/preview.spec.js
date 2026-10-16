'use strict';

// Developer preview modes (?preview=...) must not exist in the production
// build: they would render attacker-chosen text inside the real UI.

const { test, expect } = require('./fixtures');

const ID = '0123456789abcdef0123456789abcdef';
const KEY = 'A'.repeat(43);

test('?preview=secret cannot put text on the secret page', async ({ page }) => {
  await page.goto(`/secret/${ID}?preview=secret&text=SPOOF`);
  await expect(page.locator('#open-secret')).toBeVisible();
  await expect(page.locator('#view-revealed')).toBeHidden();
  await expect(page.locator('body')).not.toContainText('SPOOF');
  expect(await page.content()).not.toContain('SPOOF');
});

test('?preview=result cannot show a result panel on the create page', async ({ page }) => {
  await page.goto('/?preview=result');
  await expect(page.locator('#create-secret')).toBeVisible();
  await expect(page.locator('#result')).toBeHidden();
});

test('?preview= on the manage page runs the real status check', async ({ page }) => {
  await page.goto(`/manage/${ID}?preview=pending#${KEY}`);
  await expect(page.locator('#view-pending')).toBeHidden();
  await expect(page.locator('#view-gone')).toBeVisible();
});

test('the dev-only preview script is not served', async ({ request }) => {
  for (const path of ['/static/dev/js/preview.js', '/static/js/submitPreview.js']) {
    const res = await request.get(path);
    expect(res.status(), path).toBe(404);
  }
});
