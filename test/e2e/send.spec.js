'use strict';

// Send a secret, then open it as someone else.

const { test, expect } = require('./fixtures');
const { sendSecret, reveal, downloadFile, file } = require('./helpers');

test('a message and a file round-trip, and the link works once', async ({ page, newPerson }) => {
  const note = file('note.txt', 'attached ✓ ' + Date.now());
  const sent = await sendSecret(page, { text: 'hunter2 — ünïcödé', files: [note] });
  expect(sent.link).toMatch(/\/secret\/[0-9a-f]{32}#v1:/);

  const recipient = await newPerson();
  await recipient.goto(sent.link);
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#view-revealed')).toBeVisible();
  expect(await reveal(recipient)).toBe('hunter2 — ünïcödé');
  expect((await downloadFile(recipient, 'note.txt')).equals(note.buffer)).toBe(true);

  const late = await newPerson();
  await late.goto(sent.link);
  await late.locator('#open-secret').click();
  await expect(late.locator('#view-gone')).toBeVisible();
});

test('a passphrase is required, and a wrong one can be retried', async ({ page, newPerson, errors }) => {
  errors.allow(/consume error js\.consume\.passphrase/);
  const sent = await sendSecret(page, { text: 'two factors', passphrase: true });
  expect(sent.link).toMatch(/#v2:/);
  await expect(page.locator('#result-pass-note')).toBeVisible();

  const recipient = await newPerson();
  let claims = 0;
  recipient.on('request', function (req) {
    if (req.method() === 'GET' && /\/api\/secret\/[0-9a-f]{32}$/.test(new URL(req.url()).pathname)) claims++;
  });
  await recipient.goto(sent.link);
  await expect(recipient.locator('#open-pass-field')).toBeVisible();
  await recipient.locator('#open-passphrase').fill('wrong words entirely here now');
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#consume-error')).toBeVisible();

  await recipient.locator('#open-passphrase').fill(sent.passphrase);
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#view-revealed')).toBeVisible();
  expect(await reveal(recipient)).toBe('two factors');
  expect(claims).toBe(1);
});

test('the QR code is opt-in on the result page', async ({ page }) => {
  await sendSecret(page, { text: 'qr' });
  const qr = page.locator('#share-qr');
  await expect(qr).toBeHidden();
  await page.locator('#qr-toggle').click();
  await expect(qr).toBeVisible();
  await expect(page.locator('#share-qr-code svg, #share-qr-code canvas')).toHaveCount(1);
  await expect(page.locator('#qr-toggle')).toHaveAttribute('aria-expanded', 'true');
});
