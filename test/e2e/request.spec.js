'use strict';

// Request a secret: the requester's key stays in their browser (a
// non-extractable WebCrypto key in IndexedDB), someone else replies, and
// only the requester's browser can open the reply.

const { test, expect } = require('./fixtures');
const { reveal, downloadFile, file } = require('./helpers');

// createRequest makes a request on page and returns its reply link.
async function createRequest(page, label) {
  await page.goto('/request');
  await page.locator('#request-label').fill(label);
  await page.locator('#create-request-btn').click();
  await expect(page.locator('#view-created')).toBeVisible();
  const link = await page.locator('#reply-link').inputValue();
  expect(link).toMatch(/\/reply\/[0-9a-f]{32}#v3:/);
  return link;
}

test('a reply round-trips to the requester only', async ({ page, newPerson }) => {
  const link = await createRequest(page, 'db password');

  const replier = await newPerson();
  await replier.goto(link);
  await expect(replier.locator('#compose')).toBeVisible();
  const attachment = file('creds.txt', 'user=admin');
  await replier.locator('#secret').fill('s3cr3t reply');
  await replier.locator('#secret-files').setInputFiles([attachment]);
  await replier.locator('#create-secret button[type=submit]').click();
  await expect(replier.locator('#view-sent')).toBeVisible();

  // The created view links to the request's page, which checks on load.
  // (Going there directly skips the created view's polling interval.)
  await page.goto(await page.locator('#created-open').getAttribute('href'));
  await expect(page).toHaveURL(/\/request\/[0-9a-f]{32}$/);
  await page.locator('#open-secret').click();
  await expect(page.locator('#view-revealed')).toBeVisible();
  expect(await reveal(page)).toBe('s3cr3t reply');
  expect((await downloadFile(page, 'creds.txt')).equals(attachment.buffer)).toBe(true);

  // A request takes one reply.
  const second = await newPerson();
  await second.goto(link);
  await expect(second.locator('#view-gone')).toBeVisible();
});

test('the request list links to the request, which can be cancelled', async ({ page, newPerson }) => {
  const link = await createRequest(page, 'cancel me');

  await page.goto('/request');
  const row = page.locator('#request-list a.request-row', { hasText: 'cancel me' });
  await expect(row).toBeVisible();
  await row.click();
  await expect(page.locator('#view-waiting')).toBeVisible();
  await page.locator('#cancel-now').click();
  await expect(page.locator('#cancel-confirm')).toBeVisible();
  await page.locator('#cancel-yes').click();
  await expect(page.locator('#view-cancelled')).toBeVisible();

  const replier = await newPerson();
  await replier.goto(link);
  await expect(replier.locator('#view-gone')).toBeVisible();
});

test('another browser cannot open the request', async ({ page, newPerson }) => {
  const link = await createRequest(page, 'mine only');
  const id = new URL(link).pathname.split('/').pop();

  const stranger = await newPerson();
  await stranger.goto('/request/' + id);
  await expect(stranger.locator('#view-missing')).toBeVisible();
});
