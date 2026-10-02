'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, fastUtil, captureConsole } = require('./harness');
const { fakeResponse, installFetch } = require('./fakes');

const ID = '0123456789abcdef0123456789abcdef';
const URL_FOR_ID = 'https://gone.test/api/secret/' + ID;

// setup loads the api with instant sleeps and a registered endpoint.
function setup(t, opts) {
  reset('https://gone.test/secret/' + ID);
  t.mock.method(console, 'log', () => {});
  load('util', 'crypto');
  const delays = fastUtil();
  load('consumeApi');
  const api = window.goneConsumeApi;
  if (!opts || opts.register !== false) api.registerEndpoint(ID);
  return { api, delays, logs: captureConsole(t) };
}

const bytes = (...b) => new Uint8Array(b);
const ok = (extra) => () => fakeResponse(Object.assign({ headers: { 'Content-Length': '3', 'X-Gone-Claim': 'tok' }, chunks: [bytes(1, 2), bytes(3)] }, extra));

test('requires crypto and util; loads once', (t) => {
  reset();
  load('consumeApi');
  assert.equal(window.goneConsumeApi, undefined);
  const { api } = setup(t);
  load('consumeApi');
  assert.equal(window.goneConsumeApi, api);
});

test('FetchError and isFetchError', (t) => {
  const { api } = setup(t);
  const e = api.FetchError('m', true);
  assert.ok(e instanceof Error);
  assert.equal(e.message, 'm');
  assert.equal(e.retryable, true);
  assert.equal(api.isFetchError(e), true);
  assert.equal(api.isFetchError(new Error('x')), false);
  assert.equal(api.isFetchError(null), false);
});

test('registerEndpoint accepts only 32-char lowercase hex ids', (t) => {
  const { api } = setup(t, { register: false });
  const bad = ['', 'x', ID.toUpperCase(), ID + '0', ID.slice(1), '../' + ID.slice(3), ID.slice(0, 31) + '/', 42, null,
    '//evil.example/' + ID.slice(15)];
  for (const id of bad) {
    assert.throws(() => api.registerEndpoint(id), (e) => e.message === 'Invalid secret id' && e.retryable === false, String(id));
  }
});

test('registerEndpoint rejects a non-pinned origin', (t) => {
  const { api } = setup(t, { register: false });
  const RealURL = globalThis.URL;
  t.mock.method(globalThis, 'URL', function (p) { return new RealURL(p, 'https://evil.example'); });
  assert.throws(() => api.registerEndpoint(ID), /Invalid secret id/);
});

test('requests are blocked before an endpoint is registered', async (t) => {
  const { api, delays } = setup(t, { register: false });
  const calls = installFetch([]);
  await assert.rejects(api.fetchWithRetry({}, () => {}), (e) => e.message === 'Network error retrieving secret' && e.retryable);
  assert.equal(await api.acknowledge({ token: 't' }), false);
  assert.equal(calls.length, 0);
  // One GET retry (no claim token yet), then three ack backoffs.
  assert.deepEqual(delays, [500, 500, 1000, 1500]);
});

test('re-registering replaces the allowlisted endpoint', async (t) => {
  const { api } = setup(t);
  const other = 'ffffffffffffffffffffffffffffffff';
  api.registerEndpoint(other);
  const calls = installFetch([ok()]);
  await api.fetchWithRetry({}, () => {});
  assert.equal(calls[0].url, 'https://gone.test/api/secret/' + other);
});

test('fetchWithRetry streams the body with progress and records the claim', async (t) => {
  const { api } = setup(t);
  const calls = installFetch([ok()]);
  const progress = [];
  const claim = {};
  const out = await api.fetchWithRetry(claim, (r, total) => progress.push([r, total]));
  assert.deepEqual(out.body, bytes(1, 2, 3));
  assert.equal(out.resp.status, 200);
  assert.deepEqual(progress, [[2, 3], [3, 3]]);
  assert.equal(claim.token, 'tok');
  assert.equal(calls[0].url, URL_FOR_ID);
  assert.deepEqual(calls[0].init, {
    method: 'GET', headers: {}, mode: 'same-origin', credentials: 'same-origin',
    redirect: 'error', cache: 'no-store', keepalive: false
  });
});

test('fetchWithRetry falls back to arrayBuffer without a stream', async (t) => {
  const { api } = setup(t);
  installFetch([ok({ stream: false, headers: {} })]);
  const progress = [];
  const claim = { token: 'keep' };
  const out = await api.fetchWithRetry(claim, () => progress.push(1));
  assert.deepEqual(out.body, bytes(1, 2, 3));
  assert.deepEqual(progress, []);
  assert.equal(claim.token, 'keep');
});

test('status codes map to messages and retryability', async (t) => {
  const cases = [
    [400, /Invalid secret link/, false],
    [404, /This secret is gone/, false],
    [410, /This secret is gone/, false],
    [429, /Slow down/, false],
    [403, /Server error retrieving secret/, false]
  ];
  for (const [status, msg, retryable] of cases) {
    await t.test(String(status), async (st) => {
      const { api } = setup(st);
      const calls = installFetch([() => fakeResponse({ status })]);
      await assert.rejects(api.fetchWithRetry({}, () => {}), (e) => msg.test(e.message) && e.retryable === retryable);
      assert.equal(calls.length, 1);
    });
  }
});

test('a server error without a claim is retried once then stops', async (t) => {
  const { api, delays, logs } = setup(t);
  const calls = installFetch([() => fakeResponse({ status: 500 }), () => fakeResponse({ status: 503 })]);
  await assert.rejects(api.fetchWithRetry({}, () => {}), (e) => e.message === 'Server error retrieving secret' && e.retryable);
  assert.equal(calls.length, 2);
  assert.deepEqual(delays, [500]);
  assert.equal(logs.warn.length, 1);
});

test('retries present the claim token after a truncated or failed read', async (t) => {
  const { api, delays } = setup(t);
  const calls = installFetch([
    ok({ headers: { 'Content-Length': '9', 'X-Gone-Claim': 'tok' } }),
    ok({ readError: true }),
    ok()
  ]);
  const claim = {};
  const out = await api.fetchWithRetry(claim, () => {});
  assert.deepEqual(out.body, bytes(1, 2, 3));
  assert.equal(calls.length, 3);
  assert.deepEqual(calls[1].init.headers, { 'X-Gone-Claim': 'tok' });
  assert.deepEqual(delays, [500, 1000]);
});

test('gives up after three attempts with the last error', async (t) => {
  const { api } = setup(t);
  const trunc = ok({ headers: { 'Content-Length': '9', 'X-Gone-Claim': 'tok' } });
  const calls = installFetch([trunc, trunc, trunc]);
  await assert.rejects(api.fetchWithRetry({}, () => {}), /Download was incomplete/);
  assert.equal(calls.length, 3);
});

test('network failures are retryable', async (t) => {
  const { api } = setup(t);
  installFetch([() => { throw new TypeError('offline'); }, ok()]);
  const claim = { token: 'prev' };
  const out = await api.fetchWithRetry(claim, () => {});
  assert.equal(out.body.length, 3);
});

test('decrypt verifies version, nonce and key, then zeroes buffers', async (t) => {
  const { api } = setup(t);
  const gc = window.goneCrypto;
  const key = gc.generateKey();
  const keyB64 = gc.exportKeyB64(key);
  const enc = await gc.encrypt('hello', key);
  const headers = (v, nonce) => fakeResponse({ headers: Object.assign({ 'X-Gone-Version': v }, nonce === undefined ? {} : { 'X-Gone-Nonce': nonce }) });
  const nonce = gc.b64urlEncode(enc.nonce);

  const ct = enc.ciphertext.slice();
  const pt = await api.decrypt(headers('1', nonce), ct, keyB64);
  assert.equal(new TextDecoder().decode(pt), 'hello');
  assert.ok(ct.every((b) => b === 0));

  await assert.rejects(api.decrypt(headers('2', nonce), enc.ciphertext.slice(), keyB64), (e) => e.message === 'Unsupported secret version' && !e.retryable);
  await assert.rejects(api.decrypt(fakeResponse({}), enc.ciphertext.slice(), keyB64), /Unsupported secret version/);
  await assert.rejects(api.decrypt(headers('1', undefined), enc.ciphertext.slice(), keyB64), /Couldn.t verify/);
  await assert.rejects(api.decrypt(headers('1', nonce), enc.ciphertext.slice(), gc.exportKeyB64(gc.generateKey())), (e) => /Couldn.t verify/.test(e.message) && e.retryable === false);
  await assert.rejects(api.decrypt(headers('1', nonce), enc.ciphertext.slice(), 'AAAA'), /invalid key/);
});

test('acknowledge sends a keepalive DELETE with the claim', async (t) => {
  const { api, delays } = setup(t);
  const calls = installFetch([() => fakeResponse({ status: 204 })]);
  assert.equal(await api.acknowledge({ token: 'tok' }), true);
  assert.equal(calls[0].url, URL_FOR_ID);
  assert.equal(calls[0].init.method, 'DELETE');
  assert.equal(calls[0].init.keepalive, true);
  assert.deepEqual(calls[0].init.headers, { 'X-Gone-Claim': 'tok' });
  assert.deepEqual(delays, []);
});

test('acknowledge retries non-204 and network errors, then reports failure', async (t) => {
  const { api, delays } = setup(t);
  const calls = installFetch([
    () => fakeResponse({ status: 500 }),
    () => { throw new Error('offline'); },
    () => fakeResponse({ status: 409 })
  ]);
  assert.equal(await api.acknowledge({ token: 'tok' }), false);
  assert.equal(calls.length, 3);
  assert.deepEqual(delays, [500, 1000, 1500]);
});

test('acknowledge succeeds on a later attempt', async (t) => {
  const { api } = setup(t);
  installFetch([() => fakeResponse({ status: 500 }), () => fakeResponse({ status: 204 })]);
  assert.equal(await api.acknowledge({ token: 'tok' }), true);
});
