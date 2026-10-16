'use strict';

// Fragment hygiene: the link key (/secret/{id}#v1:...), the manage token
// (/manage/{id}#...) and the reply fill token are read once by the page, then
// removed from the address bar so they do not linger in history or session
// restore. The page keeps its in-memory copy, so the link still works; a
// reload of the cleaned URL shows the "incomplete link" state instead.

const { test, expect } = require('./fixtures');
const { sendSecret, reveal } = require('./helpers');

const NO_HASH = /^[^#]*$/;

test('the secret page drops the key from the URL and Open still works', async ({ page, newPerson }) => {
  const sent = await sendSecret(page, { text: 'fragment hygiene' });
  expect(sent.link).toMatch(/#v1:/);

  const recipient = await newPerson();
  await recipient.goto(sent.link);
  await expect(recipient).toHaveURL(NO_HASH);
  expect(new URL(recipient.url()).pathname).toBe(new URL(sent.link).pathname);
  expect(await recipient.evaluate(() => window.location.hash)).toBe('');

  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#view-revealed')).toBeVisible();
  expect(await reveal(recipient)).toBe('fragment hygiene');
});

test('reloading the cleaned secret URL says the link is incomplete, without claiming', async ({ page, newPerson }) => {
  const sent = await sendSecret(page, { text: 'still there' });
  const recipient = await newPerson();
  let claims = 0;
  recipient.on('request', (req) => { if (req.method() === 'GET' && req.url().includes('/api/secret/')) claims++; });
  await recipient.goto(sent.link);
  await expect(recipient).toHaveURL(NO_HASH);

  await recipient.reload();
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#consume-error')).toBeVisible();
  await expect(recipient.locator('#consume-error-text')).toContainText('everything after the #');
  expect(claims).toBe(0);

  // The secret was never touched. Pasting the full link into the same tab
  // changes only the fragment (a same-document navigation), and the page
  // recovers from the incomplete-link state.
  await recipient.goto(sent.link);
  await expect(recipient).toHaveURL(NO_HASH);
  await expect(recipient.locator('#consume-error')).toBeHidden();
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#view-revealed')).toBeVisible();
  expect(await reveal(recipient)).toBe('still there');
  expect(claims).toBe(1);
});

test('the manage page drops the token from the URL and keeps working', async ({ page, newPerson }) => {
  const sent = await sendSecret(page, { text: 'manage me' });
  const sender = await newPerson();
  await sender.goto(sent.manage);
  await expect(sender).toHaveURL(NO_HASH);
  await expect(sender.locator('#view-pending')).toBeVisible();

  // The token is still held in memory for the next call.
  await sender.locator('#check-again').click();
  await expect(sender.locator('#view-pending')).toBeVisible();
  await sender.locator('#delete-now').click();
  await sender.locator('#delete-yes').click();
  await expect(sender.locator('#view-deleted')).toBeVisible();

  await sender.reload();
  await expect(sender.locator('#check-error')).toBeVisible();
  await expect(sender.locator('#check-retry')).toBeHidden();
});

test('the reply page drops the key and fill token, and the reply still sends', async ({ page, newPerson }) => {
  await page.goto('/request');
  await page.locator('#request-label').fill('hygiene');
  await page.locator('#create-request-btn').click();
  await expect(page.locator('#view-created')).toBeVisible();
  const link = await page.locator('#reply-link').inputValue();
  expect(link).toMatch(/#v3:/);

  const replier = await newPerson();
  await replier.goto(link);
  await expect(replier).toHaveURL(NO_HASH);
  await expect(replier.locator('#compose')).toBeVisible();
  await replier.locator('#secret').fill('reply text');
  await replier.locator('#create-secret button[type=submit]').click();
  await expect(replier.locator('#view-sent')).toBeVisible();

  await replier.reload();
  await expect(replier.locator('#check-error')).toBeVisible();
  await expect(replier.locator('#check-retry')).toBeHidden();
});

test('pasting the manage link into the same tab recovers the invalid-link page', async ({ page, newPerson }) => {
  const sent = await sendSecret(page, { text: 'manage again' });
  const sender = await newPerson();
  await sender.goto(sent.manage);
  await expect(sender.locator('#view-pending')).toBeVisible();
  await sender.reload();
  await expect(sender.locator('#check-error')).toBeVisible();

  await sender.goto(sent.manage);
  await expect(sender).toHaveURL(NO_HASH);
  await expect(sender.locator('#view-pending')).toBeVisible();
});
