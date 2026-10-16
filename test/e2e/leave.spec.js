'use strict';

// Leaving the page: what a revealed secret leaves behind, and the warning
// before leaving while a downloaded passphrase secret is held.

const { test, expect } = require('./fixtures');
const { sendSecret, reveal, file } = require('./helpers');

// pagehide dispatches the event a browser fires when a page is put in the
// back-forward cache or unloaded. Browsers started by Playwright cannot be
// made to use the cache deterministically (Chromium turns it off, and pages
// sent no-store are not eligible anyway), so the handler is driven directly.
function pagehide(page) {
  return page.evaluate(() => window.dispatchEvent(new window.PageTransitionEvent('pagehide', { persisted: true })));
}

// beforeunloadPrevented dispatches a cancelable beforeunload and reports
// whether a handler asked the browser to warn.
function beforeunloadPrevented(page) {
  return page.evaluate(() => {
    const ev = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(ev);
    return ev.defaultPrevented;
  });
}

test('a revealed secret is gone from the page after pagehide, so a restored page shows none', async ({ page, newPerson }) => {
  const attachment = file('keep.txt', 'attached');
  const sent = await sendSecret(page, { text: 'bfcache canary text', files: [attachment] });

  const recipient = await newPerson();
  await recipient.goto(sent.link);
  await recipient.locator('#open-secret').click();
  expect(await reveal(recipient)).toBe('bfcache canary text');
  await expect(recipient.locator('#file-output-list li')).toHaveCount(1);

  await pagehide(recipient);
  await expect(recipient.locator('#view-gone')).toBeVisible();
  await expect(recipient.locator('#view-revealed')).toBeHidden();
  expect(await recipient.locator('body').textContent()).not.toContain('bfcache canary text');
  await expect(recipient.locator('#secret-output')).toHaveText('');
  await expect(recipient.locator('#file-output-list li')).toHaveCount(0);
  // Nothing left to copy or download either.
  await expect(recipient.locator('#copy-secret')).toBeHidden();
  await expect(recipient.locator('#download-all')).toBeHidden();
});

test('going back to a left secret page never shows the text', async ({ page, newPerson }) => {
  const sent = await sendSecret(page, { text: 'back-button canary' });
  const recipient = await newPerson();
  await recipient.goto(sent.link);
  await recipient.locator('#open-secret').click();
  await reveal(recipient);

  // Whether the browser restores the page from its cache (pagehide ran) or
  // loads it again (the fragment is gone, so Open is the incomplete-link
  // page), the text is not there.
  await recipient.goto('/about');
  await recipient.goBack();
  await expect(recipient.locator('#open-secret, #view-gone').first()).toBeVisible();
  expect(await recipient.locator('body').textContent()).not.toContain('back-button canary');
});

test('a downloaded passphrase secret warns before the page is left, until it is opened', async ({ page, newPerson, errors }) => {
  errors.allow(/consume error js\.consume\.passphrase/);
  const sent = await sendSecret(page, { text: 'guarded secret', passphrase: true });

  const recipient = await newPerson();
  await recipient.goto(sent.link);
  // Nothing is held before Open, so nothing to protect.
  expect(await beforeunloadPrevented(recipient)).toBe(false);

  await recipient.locator('#open-passphrase').fill('wrong words entirely here now');
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#consume-error')).toBeVisible();
  // The secret is claimed, held only in this page, still encrypted: closing
  // the page raises the browser's real "leave site?" dialog.
  expect(await beforeunloadPrevented(recipient)).toBe(true);
  const dialog = recipient.waitForEvent('dialog');
  await recipient.close({ runBeforeUnload: true });
  const warning = await dialog;
  expect(warning.type()).toBe('beforeunload');
  await warning.dismiss();
  expect(recipient.isClosed()).toBe(false);

  await recipient.locator('#open-passphrase').fill(sent.passphrase);
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#view-revealed')).toBeVisible();
  expect(await reveal(recipient)).toBe('guarded secret');
  expect(await beforeunloadPrevented(recipient)).toBe(false);
});

test('leaving erases a held passphrase secret and drops the warning', async ({ page, newPerson, errors }) => {
  errors.allow(/consume error js\.consume\.passphrase/);
  const sent = await sendSecret(page, { text: 'erase me', passphrase: true });
  const recipient = await newPerson();
  await recipient.goto(sent.link);
  await recipient.locator('#open-passphrase').fill('wrong words entirely here now');
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#consume-error')).toBeVisible();
  expect(await beforeunloadPrevented(recipient)).toBe(true);

  await pagehide(recipient);
  await expect(recipient.locator('#consume-error-text')).toContainText('You left this page');
  expect(await beforeunloadPrevented(recipient)).toBe(false);
});
