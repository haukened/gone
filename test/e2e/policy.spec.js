'use strict';

// An operator policy as a person sees it, on the "policy" server in
// servers.js: every secret needs a passphrase, and files are turned off.
// Replies to requests are exempt from the passphrase rule.

const { test, expect } = require('./fixtures');
const { servers } = require('./servers');
const { reveal } = require('./helpers');

test.use({ baseURL: servers.policy.origin });

test('the send page asks for a passphrase and offers no files', async ({ page, newPerson }) => {
  await page.goto('/');
  await expect(page.locator('#secret-files')).toHaveCount(0);
  await expect(page.locator('#drop-zone')).toHaveCount(0);
  await expect(page.locator('#pass-required')).toBeVisible();
  await expect(page.locator('#passphrase')).toBeVisible();

  await page.locator('#secret').fill('needs two factors');
  // The button is aria-disabled until there is a passphrase; pressing it
  // anyway explains why.
  await expect(page.locator('#create-secret button[type=submit]')).toHaveAttribute('aria-disabled', 'true');
  await page.locator('#create-secret button[type=submit]').click({ force: true });
  await expect(page.locator('#submit-error')).toContainText('requires a passphrase');
  await expect(page.locator('#result')).toBeHidden();

  await page.locator('#pass-generate').click();
  const passphrase = await page.locator('#passphrase').inputValue();
  await page.locator('#create-secret button[type=submit]').click();
  await expect(page.locator('#result')).toBeVisible();
  const link = await page.locator('#share-link').inputValue();
  expect(link).toMatch(/#v2:/);

  const recipient = await newPerson({ baseURL: servers.policy.origin });
  await recipient.goto(link);
  await recipient.locator('#open-passphrase').fill(passphrase);
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#view-revealed')).toBeVisible();
  expect(await reveal(recipient)).toBe('needs two factors');
});

test('the server refuses a secret without a passphrase', async ({ page }) => {
  const res = await page.request.post('/api/secret', {
    headers: { 'X-Gone-Version': '1', 'X-Gone-Nonce': 'AAAAAAAAAAAAAAAA', 'X-Gone-TTL': '15m', 'Content-Type': 'application/octet-stream' },
    data: Buffer.alloc(48, 7),
  });
  expect(res.status()).toBe(403);
  expect(await res.json()).toEqual({ error: 'passphrase required' });
  const policy = await page.request.get('/api/policy');
  expect(await policy.json()).toEqual({ passphrase_required: true, max_files: 0 });
});

test('a reply needs no passphrase and offers no files', async ({ page, newPerson }) => {
  await page.goto('/request');
  await page.locator('#request-label').fill('policy reply');
  await page.locator('#create-request-btn').click();
  await expect(page.locator('#view-created')).toBeVisible();
  const link = await page.locator('#reply-link').inputValue();

  const replier = await newPerson({ baseURL: servers.policy.origin });
  await replier.goto(link);
  await expect(replier.locator('#compose')).toBeVisible();
  await expect(replier.locator('#secret-files')).toHaveCount(0);
  await replier.locator('#secret').fill('no passphrase needed');
  await replier.locator('#create-secret button[type=submit]').click();
  await expect(replier.locator('#view-sent')).toBeVisible();

  await page.goto(await page.locator('#created-open').getAttribute('href'));
  await page.locator('#open-secret').click();
  await expect(page.locator('#view-revealed')).toBeVisible();
  expect(await reveal(page)).toBe('no passphrase needed');
});
