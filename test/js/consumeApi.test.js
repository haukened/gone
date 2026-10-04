'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { fakeResponse, installFetch } = require('./fakes');
const { ID, URL_FOR_ID, setup, bytes, ok, reset, load } = require('./consumeApiHarness');

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
  assert.equal(e.status, 0);
  assert.equal(api.FetchError('m', false, 404).status, 404);
});

test('isGone is true only for 404 and 410 fetch errors', (t) => {
  const { api } = setup(t);
  assert.equal(api.isGone(api.FetchError('m', false, 404)), true);
  assert.equal(api.isGone(api.FetchError('m', false, 410)), true);
  assert.equal(api.isGone(api.FetchError('m', true, 500)), false);
  assert.equal(api.isGone(Object.assign(new Error('x'), { status: 404 })), false);
});

test('registerEndpoint accepts only 32-char lowercase hex ids', (t) => {
  const { api } = setup(t, { register: false });
  const bad = ['', 'x', ID.toUpperCase(), ID + '0', ID.slice(1), '../' + ID.slice(3), ID.slice(0, 31) + '/', 42, null,
    '//evil.example/' + ID.slice(15)];
  for (const id of bad) {
    assert.throws(() => api.registerEndpoint(id), (e) => /isn.t valid/.test(e.message) && e.retryable === false, String(id));
  }
});

test('registerEndpoint rejects a non-pinned origin', (t) => {
  const { api } = setup(t, { register: false });
  const RealURL = globalThis.URL;
  t.mock.method(globalThis, 'URL', function (p) { return new RealURL(p, 'https://evil.example'); });
  assert.throws(() => api.registerEndpoint(ID), /isn.t valid/);
});

test('requests are blocked before an endpoint is registered', async (t) => {
  const { api, delays } = setup(t, { register: false });
  const calls = installFetch([]);
  await assert.rejects(api.fetchWithRetry({}, () => {}), (e) => /Couldn.t reach the server/.test(e.message) && e.retryable);
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
    [400, /isn.t valid/, false],
    [404, /This secret is gone/, false],
    [410, /This secret is gone/, false],
    [429, /Too many requests/, false],
    [403, /server had a problem/, false]
  ];
  for (const [status, msg, retryable] of cases) {
    await t.test(String(status), async (st) => {
      const { api } = setup(st);
      const calls = installFetch([() => fakeResponse({ status })]);
      await assert.rejects(api.fetchWithRetry({}, () => {}), (e) => msg.test(e.message) && e.retryable === retryable && e.status === status);
      assert.equal(calls.length, 1);
    });
  }
});

test('a server error without a claim is retried once then stops', async (t) => {
  const { api, delays, logs } = setup(t);
  const calls = installFetch([() => fakeResponse({ status: 500 }), () => fakeResponse({ status: 503 })]);
  await assert.rejects(api.fetchWithRetry({}, () => {}), (e) => /server is busy/.test(e.message) && e.retryable && e.status === 503);
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
  await assert.rejects(api.fetchWithRetry({}, () => {}), /download was interrupted/);
  assert.equal(calls.length, 3);
});

test('Content-Length must be canonical and match exactly when present', async (t) => {
  const cases = [
    ['too long', { 'Content-Length': '2' }, true],
    ['non-canonical', { 'Content-Length': '03' }, false],
    ['not a number', { 'Content-Length': 'abc' }, false]
  ];
  for (const [name, headers, cancels] of cases) {
    await t.test(name, async (st) => {
      const { api } = setup(st);
      const responses = [];
      const once = () => { const r = ok({ headers })(); responses.push(r); return r; };
      installFetch([once, once, once]);
      await assert.rejects(api.fetchWithRetry({}, () => {}), (e) => e.message === 'The download was interrupted.' && e.retryable);
      assert.equal(Boolean(responses[0].cancelled), cancels);
    });
  }
});

test('a missing Content-Length accepts whatever arrives', async (t) => {
  const { api } = setup(t);
  const progress = [];
  installFetch([ok({ headers: { 'X-Gone-Claim': 'tok' } })]);
  const out = await api.fetchWithRetry({}, (r, total) => progress.push([r, total]));
  assert.deepEqual(out.body, bytes(1, 2, 3));
  assert.deepEqual(progress, [[2, -1], [3, -1]]);
});

test('network failures are retryable', async (t) => {
  const { api } = setup(t);
  installFetch([() => { throw new TypeError('offline'); }, ok()]);
  const claim = { token: 'prev' };
  const out = await api.fetchWithRetry(claim, () => {});
  assert.equal(out.body.length, 3);
});

