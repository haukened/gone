'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');
const { installFetch } = require('./fakes');

// tr renders a message key (as carried by errors) in English.
const tr = (k) => window.goneI18n.t(k);

const ID = '0123456789abcdef0123456789abcdef';
const TOKEN = 'aZ09_-'.repeat(7) + 'x';
const STATUS_URL = `https://gone.test/api/secret/${ID}/status`;
const REVOKE_URL = `https://gone.test/api/secret/${ID}/revoke`;

// resp builds a minimal fetch response; body is returned by json() or,
// when it is an Error, thrown from it.
function resp(status, body) {
  return () => ({
    status: status,
    json: async () => {
      if (body instanceof Error) throw body;
      return body;
    }
  });
}

function setup() {
  reset('https://gone.test/manage/' + ID);
  load('manageApi');
  return window.goneManageApi;
}

test('loads once and exposes a frozen API', () => {
  const api = setup();
  load('manageApi');
  assert.equal(window.goneManageApi, api);
  assert.ok(Object.isFrozen(api));
  assert.match(tr(api.INVALID_LINK), /isn\u2019t valid/);
});

test('isToken accepts only 43-char base64url', () => {
  const api = setup();
  assert.equal(api.isToken(TOKEN), true);
  for (const bad of [TOKEN + 'a', TOKEN.slice(1), 'A'.repeat(42) + '=', 'A'.repeat(42) + '+', '', undefined, 42]) {
    assert.equal(api.isToken(bad), false, String(bad));
  }
});

test('ManageError and isManageError', () => {
  const api = setup();
  const e = api.ManageError('m', true);
  assert.ok(e instanceof Error);
  assert.equal(e.message, 'm');
  assert.equal(api.isManageError(e), true);
  assert.equal(api.isManageError(api.ManageError('x', false)), true);
  assert.equal(api.isManageError(new Error('x')), false);
  assert.equal(api.isManageError(null), false);
});

test('registerEndpoint rejects malformed ids and pins same-origin URLs', async () => {
  const api = setup();
  for (const bad of [ID.toUpperCase(), ID + '0', '../x', '', undefined, 7]) {
    assert.throws(() => api.registerEndpoint(bad), (e) => api.isManageError(e) && !e.retryable && e.message === api.INVALID_LINK);
  }
  const calls = installFetch([resp(404), resp(204)]);
  api.registerEndpoint(ID);
  await api.status(TOKEN);
  await api.revoke(TOKEN);
  assert.deepEqual(calls.map((c) => c.url), [STATUS_URL, REVOKE_URL]);
});

test('requests are refused before registration or with a bad token', async () => {
  const api = setup();
  const calls = installFetch([]);
  await assert.rejects(api.status(TOKEN), (e) => /unexpected address/.test(tr(e.message)));
  api.registerEndpoint(ID);
  await assert.rejects(api.revoke('nope'), (e) => e.message === api.INVALID_LINK && !e.retryable);
  assert.equal(calls.length, 0);
});

test('re-registering replaces the allowlist', async () => {
  const api = setup();
  const other = 'f'.repeat(32);
  api.registerEndpoint(other);
  api.registerEndpoint(ID);
  const calls = installFetch([resp(404)]);
  await api.status(TOKEN);
  assert.equal(calls[0].url, STATUS_URL);
});

test('status sends the token in a header with locked-down fetch options', async () => {
  const api = setup();
  api.registerEndpoint(ID);
  const calls = installFetch([resp(200, { state: 'pending', created_at: '2030-01-01T00:00:00Z', expires_at: '2030-01-02T00:00:00Z' })]);
  const out = await api.status(TOKEN);
  assert.equal(out.state, 'pending');
  assert.equal(out.createdAt.toISOString(), '2030-01-01T00:00:00.000Z');
  assert.equal(out.expiresAt.toISOString(), '2030-01-02T00:00:00.000Z');
  const init = calls[0].init;
  assert.equal(init.method, 'GET');
  assert.deepEqual(init.headers, { 'X-Gone-Manage': TOKEN });
  assert.equal(init.mode, 'same-origin');
  assert.equal(init.credentials, 'same-origin');
  assert.equal(init.redirect, 'error');
  assert.equal(init.cache, 'no-store');
  assert.equal(init.keepalive, false);
  assert.equal(calls[0].url.includes(TOKEN), false);
});

test('status maps 404 to gone and validates the pending body', async () => {
  const api = setup();
  api.registerEndpoint(ID);
  installFetch([resp(404)]);
  assert.deepEqual(await api.status(TOKEN), { state: 'gone' });
  const bodies = [
    new SyntaxError('bad json'), null, { state: 'gone' },
    { state: 'pending', created_at: 'nope', expires_at: '2030-01-01T00:00:00Z' },
    { state: 'pending', created_at: '2030-01-01T00:00:00Z', expires_at: 5 }
  ];
  installFetch(bodies.map((b) => resp(200, b)));
  for (const b of bodies) {
    await assert.rejects(api.status(TOKEN), (e) => e.retryable && /server had a problem/.test(tr(e.message)), String(b));
  }
});

test('status and revoke map failure statuses', async () => {
  const api = setup();
  api.registerEndpoint(ID);
  const cases = [[400, /isn\u2019t valid/, false], [429, /Too many requests/, true], [503, /server is busy/, true], [500, /server had a problem/, true], [403, /server had a problem/, true]];
  installFetch(cases.flatMap(([s]) => [resp(s), resp(s)]));
  for (const [s, re, retryable] of cases) {
    await assert.rejects(api.status(TOKEN), (e) => re.test(tr(e.message)) && e.retryable === retryable, `status ${s}`);
    await assert.rejects(api.revoke(TOKEN), (e) => re.test(tr(e.message)) && e.retryable === retryable, `revoke ${s}`);
  }
});

test('network failures are retryable', async () => {
  const api = setup();
  api.registerEndpoint(ID);
  const boom = () => { throw new TypeError('Failed to fetch'); };
  installFetch([boom, boom]);
  await assert.rejects(api.status(TOKEN), (e) => e.retryable && /Couldn\u2019t reach/.test(tr(e.message)));
  await assert.rejects(api.revoke(TOKEN), (e) => e.retryable && /Couldn\u2019t reach/.test(tr(e.message)));
});

test('revoke posts with keepalive and maps 204 and 404', async () => {
  const api = setup();
  api.registerEndpoint(ID);
  const calls = installFetch([resp(204), resp(404)]);
  assert.equal(await api.revoke(TOKEN), 'deleted');
  assert.equal(await api.revoke(TOKEN), 'gone');
  assert.equal(calls[0].init.method, 'POST');
  assert.equal(calls[0].init.keepalive, true);
  assert.deepEqual(calls[0].init.headers, { 'X-Gone-Manage': TOKEN });
});
