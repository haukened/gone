'use strict';

// The website, built (task site) and served as GitHub Pages serves it: every
// page loads with no errors and nothing from another origin, passes axe,
// and the interactive parts (install box, theme, docs search and menus) work.

const { test, expect } = require('@playwright/test');
const { default: AxeBuilder } = require('@axe-core/playwright');

const PAGES = [
  '/',
  '/why/',
  '/choose/',
  '/trust/',
  '/privacy/',
  '/docs/',
  '/docs/self-host/',
  '/docs/self-host/compose/',
  '/docs/configuration/',
  '/docs/cli-reference/',
  '/docs/security/',
  '/docs/faq/',
];

// watch collects console errors (CSP violations included), page errors and
// requests to any other origin.
function watch(page, origin) {
  const problems = [];
  page.on('console', (m) => { if (m.type() === 'error') problems.push('console: ' + m.text()); });
  page.on('pageerror', (e) => problems.push('page error: ' + e.message));
  page.on('request', (r) => {
    const url = r.url();
    if (!url.startsWith(origin) && !url.startsWith('data:')) problems.push('foreign request: ' + url);
  });
  return problems;
}

for (const path of PAGES) {
  test(`${path} loads cleanly and passes axe in both themes`, async ({ page, baseURL }) => {
    const problems = watch(page, baseURL);
    const res = await page.goto(path, { waitUntil: 'networkidle' });
    expect(res.status()).toBe(200);
    // axe in both themes: contrast differs between them.
    for (const scheme of ['light', 'dark']) {
      await page.emulateMedia({ colorScheme: scheme });
      // Let the theme's colour transitions (0.25s) finish before measuring.
      await page.waitForTimeout(500);
      const axe = await new AxeBuilder({ page }).analyze();
      const violations = axe.violations.map((v) => `${scheme} ${v.id} (${v.impact}): ${v.nodes.map((n) => n.target.join(' ')).slice(0, 3).join(', ')}`);
      expect(violations).toEqual([]);
    }
    expect(problems).toEqual([]);
  });
}

test('a missing page answers 404 with the gone page', async ({ page }) => {
  const res = await page.goto('/no/such/page/');
  expect(res.status()).toBe(404);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('This page is gone.');
  await page.getByRole('link', { name: 'Go to the home page' }).click();
  await expect(page).toHaveURL(/\/$/);
});

test('the install box copies the command', async ({ page, browserName, context }) => {
  test.skip(browserName !== 'chromium', 'clipboard permissions are Chromium-only in Playwright');
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.goto('/');
  const command = (await page.locator('.install .tab-panel:not([hidden]) code').first().textContent()).trim();
  await page.locator('.install .tab-panel:not([hidden]) button[data-copy]').first().click();
  await expect(page.locator('.install .tab-panel:not([hidden]) button[data-copy]').first()).toContainText('Copied');
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(command);
});

test('the install tabs follow the arrow keys', async ({ page }) => {
  await page.goto('/');
  const tabs = page.getByRole('tab');
  await tabs.first().focus();
  await page.keyboard.press('ArrowRight');
  await expect(tabs.nth(1)).toHaveAttribute('aria-selected', 'true');
  await expect(tabs.nth(1)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(tabs.first()).toHaveAttribute('aria-selected', 'true');
});

test('the theme choice survives a reload', async ({ page }) => {
  await page.goto('/');
  const html = page.locator('html');
  const before = (await html.getAttribute('data-theme')) || '';
  await page.locator('#theme-toggle').click();
  const after = await html.getAttribute('data-theme');
  expect(after).not.toBe(before);
  await page.reload();
  await expect(html).toHaveAttribute('data-theme', after);
});

test('docs search finds a variable and goes to its row', async ({ page }) => {
  await page.goto('/docs/self-host/compose/');
  await page.keyboard.press('/');
  await page.getByRole('combobox', { name: 'Search the docs' }).fill('trusted proxies');
  const first = page.locator('#search-results a').first();
  await expect(first).toHaveAttribute('href', '/docs/configuration/#gone_trusted_proxies');
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/\/docs\/configuration\/#gone_trusted_proxies$/);
  await expect(page.locator('#gone_trusted_proxies')).toBeInViewport();
});

test('on a phone the docs menu opens, and closes when a page is chosen', async ({ browser, baseURL }) => {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, baseURL });
  const page = await context.newPage();
  await page.goto('/docs/self-host/compose/');
  const menu = page.locator('.docs-drop-menu');
  await menu.locator('> summary').click();
  await expect(menu).toHaveAttribute('open', '');
  await menu.getByRole('link', { name: 'Docker', exact: true }).click();
  await expect(page).toHaveURL(/\/docs\/self-host\/docker\/$/);
  await context.close();
});

test('security.txt and robots.txt are served', async ({ request }) => {
  const sec = await (await request.get('/.well-known/security.txt')).text();
  expect(sec).toContain('Contact: https://github.com/haukened/gone/security/advisories/new');
  const expires = new Date(sec.match(/^Expires: (.+)$/m)[1]);
  expect(expires.getTime()).toBeGreaterThan(Date.now());
  const robots = await (await request.get('/robots.txt')).text();
  expect(robots).toContain('Sitemap: ');
});
