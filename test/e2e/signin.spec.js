'use strict';

// Sign-in for private instances (GONE_AUTH), on two servers from servers.js:
//   signin: a signed OIDC token, which the proxy mints the way oauth2-proxy
//           passes an identity provider's token through.
//   custom: a plain X-Gone-User header from the trusted proxy, with a
//           sign-in button pointing at the proxy's stub login page.
// The proxy only vouches for a browser after /e2e-login set its cookie, and
// it strips identity headers the browser sends itself.

const { test, expect } = require('./fixtures');
const { servers, plain } = require('./servers');
const { sendSecret, reveal } = require('./helpers');

test.skip(plain, 'sign-in needs the TLS proxy, which stands in for the sign-in proxy');

const CREATE = { 'X-Gone-Version': '1', 'X-Gone-Nonce': 'AAAAAAAAAAAAAAAA', 'X-Gone-TTL': '15m', 'Content-Type': 'application/octet-stream' };

// signIn visits the stub login page, then waits until the server shows the
// send form: the signin server may still be loading the issuer's keys.
async function signIn(page) {
  await page.goto('/e2e-login?rd=/');
  await expect.poll(async function () {
    await page.reload();
    return page.locator('#create-secret').count();
  }, { timeout: 30000, intervals: [250, 500, 1000] }).toBe(1);
}

test.describe('signed token (oidc)', () => {
  test.skip(process.platform === 'darwin', 'goned trusts the stand-in issuer through SSL_CERT_FILE, which Go ignores on macOS');
  test.use({ baseURL: servers.signin.origin });

  test('signed out: the pages ask to sign in and the API refuses', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#sign-in')).toBeVisible();
    await expect(page.locator('#sign-in-heading')).toHaveText('Sign in required');
    await expect(page.locator('#sign-in-link')).toHaveCount(0);
    await expect(page.locator('#create-secret')).toHaveCount(0);
    await page.goto('/request');
    await expect(page.locator('#sign-in')).toBeVisible();
    await expect(page.locator('#create-request')).toHaveCount(0);

    for (const path of ['/api/secret', '/api/request']) {
      const res = await page.request.post(path, { headers: CREATE, data: Buffer.alloc(48, 7) });
      expect(res.status(), path).toBe(401);
    }
    const forged = await page.request.post('/api/secret', { headers: { ...CREATE, Authorization: 'Bearer forged' }, data: Buffer.alloc(48, 7) });
    expect(forged.status()).toBe(401);
    expect((await (await page.request.get('/api/policy')).json()).sign_in_required).toBe(true);
  });

  test('signed in: sending works, and the recipient opens it without signing in', async ({ page, newPerson }) => {
    await signIn(page);
    const sent = await sendSecret(page, { text: 'behind sign-in' });
    const recipient = await newPerson();
    await recipient.goto(sent.link);
    await recipient.locator('#open-secret').click();
    await expect(recipient.locator('#view-revealed')).toBeVisible();
    expect(await reveal(recipient)).toBe('behind sign-in');
  });

  test('a request is answered by someone who is not signed in', async ({ page, newPerson }) => {
    await signIn(page);
    await page.goto('/request');
    await page.locator('#create-request-btn').click();
    await expect(page.locator('#view-created')).toBeVisible();
    const link = await page.locator('#reply-link').inputValue();

    const replier = await newPerson();
    await replier.goto(link);
    await expect(replier.locator('#compose')).toBeVisible();
    await replier.locator('#secret').fill('from outside');
    await replier.locator('#create-secret button[type=submit]').click();
    await expect(replier.locator('#view-sent')).toBeVisible();

    await page.goto(await page.locator('#created-open').getAttribute('href'));
    await page.locator('#open-secret').click();
    await expect(page.locator('#view-revealed')).toBeVisible();
    expect(await reveal(page)).toBe('from outside');
  });
});

test.describe('plain header (custom)', () => {
  test.use({ baseURL: servers.custom.origin });

  test('the sign-in button goes to the login page and back to the form', async ({ page }) => {
    for (const [path, form] of [['/', '#create-secret'], ['/request', '#create-request']]) {
      await page.context().clearCookies();
      await page.goto(path);
      const button = page.locator('#sign-in-link');
      await expect(button).toHaveText('Sign in with your identity provider');
      await button.click();
      await expect(page).toHaveURL(new RegExp(path === '/' ? '/$' : '/request$'));
      await expect(page.locator(form)).toBeVisible();
    }
  });

  test('a header the browser sends itself is stripped by the proxy', async ({ page }) => {
    const res = await page.request.post('/api/secret', { headers: { ...CREATE, 'X-Gone-User': 'mallory@example.com' }, data: Buffer.alloc(48, 7) });
    expect(res.status()).toBe(401);
  });
});
