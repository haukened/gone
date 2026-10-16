'use strict';

// Languages: server-rendered in each locale, and switched live in the page.

const fs = require('node:fs');
const path = require('node:path');
const { test, expect } = require('./fixtures');

const LOCALES = ['en', 'es', 'fr', 'de', 'pt-BR'];

// message returns key from a locale's catalog in web/messages.
function message(locale, key) {
  const file = path.join(__dirname, '../../web/messages', locale + '.json');
  return JSON.parse(fs.readFileSync(file, 'utf8'))[key];
}

for (const locale of LOCALES) {
  test(`the send page renders in ${locale}`, async ({ page, context, baseURL }) => {
    await context.addCookies([{ name: 'gone_lang', value: locale, url: baseURL }]);
    await page.goto('/');
    await expect(page.locator('html')).toHaveAttribute('lang', locale);
    await expect(page.locator('[data-i18n="send.eyebrow"]')).toHaveText(message(locale, 'send.eyebrow'));
  });
}

test('the picker switches language in place and remembers it', async ({ page, context }) => {
  await page.goto('/');
  await page.evaluate(function () { window.__noReload = true; });

  await page.locator('#lang-button').click();
  await page.locator('#lang-menu button.lang-option[data-lang="de"]').click();
  await expect(page.locator('html')).toHaveAttribute('lang', 'de');
  await expect(page.locator('[data-i18n="send.eyebrow"]')).toHaveText(message('de', 'send.eyebrow'));
  expect(await page.evaluate(function () { return window.__noReload; })).toBe(true);

  const cookie = (await context.cookies()).find(function (c) { return c.name === 'gone_lang'; });
  expect(cookie && cookie.value).toBe('de');
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('lang', 'de');
});

test('the picker works from the keyboard', async ({ page }) => {
  await page.goto('/');
  const button = page.locator('#lang-button');
  const options = page.locator('#lang-menu button.lang-option');
  await button.focus();
  await page.keyboard.press('ArrowDown');
  await expect(page.locator('#lang-menu')).toBeVisible();
  await expect(button).toHaveAttribute('aria-expanded', 'true');

  await page.keyboard.press('End');
  await expect(options.last()).toBeFocused();
  await page.keyboard.press('Home');
  await expect(options.first()).toBeFocused();

  await page.keyboard.press('Escape');
  await expect(page.locator('#lang-menu')).toBeHidden();
  await expect(button).toBeFocused();
});
