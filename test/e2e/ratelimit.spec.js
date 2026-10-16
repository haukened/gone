'use strict';

// Rate limiting as a person sees it. These run against their own server (the
// "rate" server in servers.js): a burst of 3, and a budget of 30 a minute, so
// a token comes back every two seconds. The rest of the suite runs with the
// default limits and a burst of 1000 so it never trips them.
//
// Every browser connects from 127.0.0.1, so each test sends its own
// X-Forwarded-For (the server trusts the proxy in front of it) and gets a
// budget of its own, however the specs are scheduled.

const { test, expect } = require('./fixtures');
const { servers } = require('./servers');

// Rate limiting is server behaviour seen through the UI, so one browser is
// enough; WebKit under CI load has reported the 429 message late.
test.skip(({ browserName }) => browserName !== 'chromium', 'rate limiting is server behaviour; one browser is enough');

const BURST = servers.rate.rate.burst;
const BROWSERS = ['chromium', 'firefox', 'webkit'];

// client returns context options for a person on the rate server whose
// address is unique to this test and browser.
function client(testInfo, slot) {
  const browser = Math.max(BROWSERS.indexOf(testInfo.project.name), 0);
  return { baseURL: servers.rate.origin, extraHTTPHeaders: { 'X-Forwarded-For': `10.77.${browser}.${slot}` } };
}

// drain makes requests until the server answers 429, and returns that
// response. Whatever the clock does, it stops once the budget is gone.
async function drain(send) {
  for (let i = 0; i <= BURST + 2; i++) {
    const res = await send();
    if (res.status() === 429) return res;
  }
  throw new Error('the server never rate limited the client');
}

test('creating past the budget shows the too-many-requests message, and works again after Retry-After', async ({ newPerson, errors }, testInfo) => {
  errors.allow(/server error 429|upload failed/);
  const person = await newPerson(client(testInfo, 1));
  await person.goto('/');
  await person.locator('#secret').fill('rate limited, then not');
  await person.locator('#secret').evaluate((el) => el.dispatchEvent(new Event('input', { bubbles: true })));

  // Spend the create budget with the API (a ciphertext's bytes are never
  // looked at), so the form's send is the one that is turned away.
  const res = await drain(() => person.request.post('/api/secret', {
    headers: { 'X-Gone-Version': '1', 'X-Gone-Nonce': 'AAAAAAAAAAAAAAAA', 'X-Gone-TTL': '15m', 'Content-Type': 'application/octet-stream' },
    data: Buffer.alloc(48, 7),
  }));
  const retryAfter = Number(res.headers()['retry-after']);
  expect(retryAfter).toBeGreaterThanOrEqual(1);

  await person.locator('#create-secret button[type=submit]').click();
  const error = person.locator('#submit-error');
  await expect(error).toBeVisible();
  await expect(error).toContainText('Too many requests right now');
  await expect(person.locator('#result')).toBeHidden();

  // The advertised wait is enough: the same form then goes through.
  await person.waitForTimeout((retryAfter + 1) * 1000);
  await person.locator('#create-secret button[type=submit]').click();
  await expect(person.locator('#result')).toBeVisible();
  await expect(person.locator('#share-link')).toHaveValue(/\/secret\/[0-9a-f]{32}#v1:/);
});

test('opening past the read budget shows the too-many-requests message, and works again later', async ({ newPerson, errors }, testInfo) => {
  errors.allow(/consume error js\.common\.tooMany/);
  const person = await newPerson(client(testInfo, 2));
  // A link to nothing: the claim answers 404 once it is let through.
  const id = '0123456789abcdef0123456789abcdef';
  await person.goto(`/secret/${id}#v1:${'A'.repeat(43)}`);

  const res = await drain(() => person.request.get(`/api/secret/${id}/status`, { headers: { 'X-Gone-Manage': 'A'.repeat(43) } }));
  const retryAfter = Number(res.headers()['retry-after']);
  expect(retryAfter).toBeGreaterThanOrEqual(1);

  await person.locator('#open-secret').click();
  await expect(person.locator('#consume-error')).toBeVisible();
  await expect(person.locator('#consume-error-text')).toContainText('Too many requests right now');

  await person.waitForTimeout((retryAfter + 1) * 1000);
  await person.locator('#open-secret').click();
  await expect(person.locator('#view-gone')).toBeVisible();
});
