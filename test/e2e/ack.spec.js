'use strict';

// The acknowledge call (DELETE /api/secret/{id}) fails or says the secret is
// already deleted. The DELETE is faulted in the browser with page.route; the
// claim before it goes to the real server.

const { test, expect } = require('./fixtures');
const { sendSecret, reveal } = require('./helpers');

const ACK = /\/api\/secret\/[0-9a-f]{32}$/;

// faultAck answers the first statuses.length DELETEs with those statuses and
// lets later ones through. It returns the list of DELETE attempts.
async function faultAck(page, statuses) {
  const attempts = [];
  await page.route(ACK, async (route) => {
    if (route.request().method() !== 'DELETE') {
      await route.continue();
      return;
    }
    attempts.push(route.request().headers()['x-gone-claim']);
    const status = statuses[attempts.length - 1];
    if (status) await route.fulfill({ status: status, body: '' });
    else await route.continue();
  });
  return attempts;
}

// openSecret sends a secret and opens it as another person, with the
// acknowledge call faulted as statuses says.
async function openSecret(page, newPerson, text, statuses) {
  const sent = await sendSecret(page, { text: text });
  const recipient = await newPerson();
  const attempts = await faultAck(recipient, statuses);
  await recipient.goto(sent.link);
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#view-revealed')).toBeVisible();
  return { recipient, attempts, sent };
}

test('a 5xx on the acknowledge is retried, and the retry succeeding shows no warning', async ({ page, newPerson }) => {
  const { recipient, attempts, sent } = await openSecret(page, newPerson, 'ack retry', [503]);
  expect(await reveal(recipient)).toBe('ack retry');
  await expect.poll(() => attempts.length).toBe(2);
  await expect(recipient.locator('#ack-warning')).toBeHidden();
  // The same claim token was presented both times, and the retry really
  // deleted the secret on the server.
  expect(attempts[1]).toBe(attempts[0]);
  const late = await newPerson();
  await late.goto(sent.link);
  await late.locator('#open-secret').click();
  await expect(late.locator('#view-gone')).toBeVisible();
});

test('an acknowledge that keeps failing warns that deletion was not confirmed', async ({ page, newPerson }) => {
  const { recipient, attempts } = await openSecret(page, newPerson, 'ack warn', [500, 502, 503]);
  expect(await reveal(recipient)).toBe('ack warn');
  await expect(recipient.locator('#ack-warning')).toBeVisible();
  await expect(recipient.locator('#ack-warning')).toContainText('Couldn’t confirm deletion');
  expect(attempts).toHaveLength(3);
});

test('a 404 on the acknowledge counts as deleted: no warning, no retry', async ({ page, newPerson }) => {
  const { recipient, attempts } = await openSecret(page, newPerson, 'ack 404', [404]);
  expect(await reveal(recipient)).toBe('ack 404');
  await expect(recipient.locator('#ack-warning')).toBeHidden();
  expect(attempts).toHaveLength(1);
});
