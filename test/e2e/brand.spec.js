'use strict';

// Operator branding as a visitor sees it, on the "brand" server in
// servers.js: the operator's logo, name and accent in the header, their
// links in the footer, and "Powered by Gone" kept.

const { test, expect } = require('./fixtures');
const { servers } = require('./servers');

test.use({ baseURL: servers.brand.origin });

const NAME = 'Acme, Inc. Secrets';

test('the header shows the operator logo and name', async ({ page }) => {
  await page.goto('/');
  const home = page.locator('header a.brand');
  await expect(home).toHaveAccessibleName(`${NAME}, home`);
  await expect(home.locator('.brand-name')).toHaveText(NAME);
  await expect(page.locator('header .brand-g')).toHaveCount(0);

  const logo = home.locator('img.brand-logo');
  await expect(logo).toBeVisible();
  expect(await logo.evaluate((img) => img.complete && img.naturalWidth > 0)).toBe(true);
});

test('the footer links to the operator pages and keeps Powered by Gone', async ({ page }) => {
  await page.goto('/');
  const foot = page.locator('footer nav');
  await expect(foot.getByRole('link', { name: 'Contact' })).toHaveAttribute('href', 'mailto:it@example.com');
  await expect(foot.getByRole('link', { name: 'Imprint' })).toHaveAttribute('href', 'https://example.com/imprint');
  await expect(foot.getByRole('link', { name: 'Privacy' })).toHaveAttribute('href', 'https://example.com/privacy');
  await expect(foot.locator('.ver')).toContainText('Powered by Gone');
});

test('the footer and home label follow a language switch', async ({ page }) => {
  await page.goto('/');
  await page.locator('#lang-button').click();
  await page.locator('.lang-option[data-lang="de"]').click();
  await expect(page.locator('footer nav').getByRole('link', { name: 'Impressum' })).toBeVisible();
  await expect(page.locator('header a.brand')).toHaveAccessibleName(`${NAME}, Startseite`);
  await expect(page.locator('footer .ver')).toContainText('Betrieben mit Gone');
});

for (const [scheme, rgb] of [['light', 'rgb(29, 78, 216)'], ['dark', 'rgb(147, 197, 253)']]) {
  test(`the accent uses the operator colour in the ${scheme} theme`, async ({ newPerson }) => {
    const person = await newPerson({ baseURL: servers.brand.origin, colorScheme: scheme });
    await person.goto('/');
    const accent = person.locator('h1 .accent').first();
    await expect(accent).toHaveCSS('color', rgb);
  });
}
