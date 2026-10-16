'use strict';

// Page actions shared by the specs.

const { expect } = require('./fixtures');

// sendSecret fills in and submits the send form on page.
//
// opts.text is the message, opts.files a list of {name, mimeType, buffer},
// and opts.passphrase true to generate one.
//
// It returns the share link, the manage link and the passphrase ('' if none).
async function sendSecret(page, opts) {
  await page.goto('/');
  await page.locator('#secret').fill(opts.text || '');
  if (opts.files && opts.files.length) {
    await page.locator('#secret-files').setInputFiles(opts.files);
    await expect(page.locator('#file-list li')).toHaveCount(opts.files.length);
  }
  let passphrase = '';
  if (opts.passphrase) {
    await page.locator('#pass-disclosure summary').click();
    await page.locator('#pass-generate').click();
    passphrase = await page.locator('#passphrase').inputValue();
    // Five CamelCase words from the EFF short wordlist.
    expect(passphrase).toMatch(/^([A-Z][a-z]+){5}$/);
  }
  await page.locator('#create-secret button[type=submit]').click();
  await expect(page.locator('#result')).toBeVisible();
  const link = await page.locator('#share-link').inputValue();
  await page.locator('#manage-disclosure summary').click();
  const manage = await page.locator('#manage-link').inputValue();
  return { link: link, manage: manage, passphrase: passphrase };
}

// reveal clicks Show on an opened secret and returns the message text.
async function reveal(page) {
  await page.locator('#show-secret').click();
  const out = page.locator('#secret-output');
  await expect(out).toBeVisible();
  return out.textContent();
}

// downloadFile clicks the Download button for name and returns its bytes.
async function downloadFile(page, name) {
  const row = page.locator('#file-output-list li', { hasText: name });
  const [download] = await Promise.all([
    page.waitForEvent('download'),
    row.getByRole('button').click(),
  ]);
  expect(download.suggestedFilename()).toBe(name);
  const stream = await download.createReadStream();
  const chunks = [];
  for await (const chunk of stream) chunks.push(chunk);
  return Buffer.concat(chunks);
}

// file returns a small test attachment.
function file(name, text) {
  return { name: name, mimeType: 'text/plain', buffer: Buffer.from(text) };
}

module.exports = { sendSecret, reveal, downloadFile, file };
