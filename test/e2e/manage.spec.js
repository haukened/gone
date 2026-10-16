'use strict';

// The sender's manage page: check status, then delete before it's opened.

const { test, expect } = require('./fixtures');
const { sendSecret } = require('./helpers');

test('the sender can check a secret and delete it', async ({ page, newPerson }) => {
  const sent = await sendSecret(page, { text: 'delete me' });

  const sender = await newPerson();
  await sender.goto(sent.manage);
  await expect(sender.locator('#view-pending')).toBeVisible();
  await sender.locator('#check-again').click();
  await expect(sender.locator('#view-pending')).toBeVisible();

  // The inline confirm can be backed out of.
  await sender.locator('#delete-now').click();
  await expect(sender.locator('#delete-confirm')).toBeVisible();
  await sender.locator('#delete-no').click();
  await expect(sender.locator('#delete-confirm')).toBeHidden();

  await sender.locator('#delete-now').click();
  await sender.locator('#delete-yes').click();
  await expect(sender.locator('#view-deleted')).toBeVisible();

  const recipient = await newPerson();
  await recipient.goto(sent.link);
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#view-gone')).toBeVisible();
});

test('the manage page says gone once the secret is opened', async ({ page, newPerson }) => {
  const sent = await sendSecret(page, { text: 'read me' });

  const recipient = await newPerson();
  await recipient.goto(sent.link);
  await recipient.locator('#open-secret').click();
  await expect(recipient.locator('#view-revealed')).toBeVisible();

  const sender = await newPerson();
  await sender.goto(sent.manage);
  await expect(sender.locator('#view-gone')).toBeVisible();
});
