'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');
const { fakeResponse, installFetch } = require('./fakes');

const ID = 'a'.repeat(32);
const TOKEN = 'T'.repeat(43);
const FILL = 'F'.repeat(43);
const json = (status, body, headers) => () => Object.assign(fakeResponse({ status, headers }), { json: async () => body });

function api() {
  reset('https://gone.test/request');
  load('requestApi');
  return window.goneRequestApi;
}

test('create posts the TTL and validates the response', async () => {
  const a = api();
  const calls = installFetch([json(201, { id: ID, expires_at: '2026-01-01T00:00:00Z', manage_token: TOKEN, fill_token: FILL })]);
  const got = await a.create('30m');
  assert.equal(calls[0].url, 'https://gone.test/api/request');
  assert.equal(calls[0].init.method, 'POST');
  assert.equal(calls[0].init.headers['X-Gone-TTL'], '30m');
  assert.equal(calls[0].init.redirect, 'error');
  assert.deepEqual([got.id, got.manageToken, got.fillToken], [ID, TOKEN, FILL]);
  assert.equal(got.expiresAt.toISOString(), '2026-01-01T00:00:00.000Z');
});

test('create rejects bad responses', async () => {
  const a = api();
  const bad = [
    json(201, { id: 'x', manage_token: TOKEN, fill_token: FILL, expires_at: '2026-01-01T00:00:00Z' }),
    json(201, { id: ID, manage_token: TOKEN, fill_token: FILL, expires_at: 'never' }),
    json(201, null),
    () => Object.assign(fakeResponse({ status: 201 }), { json: async () => { throw new Error('bad'); } }),
    json(400, {}),
    json(429, {}, { 'Retry-After': '7' }),
    json(503, {}, { 'Retry-After': 'soon' }),
    json(500, {}),
    () => { throw new Error('offline'); }
  ];
  installFetch(bad);
  const results = [];
  for (let i = 0; i < bad.length; i++) results.push(await a.create('1h').catch((e) => e));
  results.forEach((e) => assert.ok(a.isRequestError(e)));
  assert.equal(results[4].retryable, false);
  assert.equal(results[5].retryAfter, 7);
  assert.equal(results[6].retryAfter, 0);
  assert.match(window.goneI18n.t(results[8].message), /reach the server/);
});

test('status maps waiting, ready and gone', async () => {
  const a = api();
  const body = (state) => json(200, { state, created_at: '2026-01-01T00:00:00Z', expires_at: '2026-01-01T01:00:00Z' });
  const calls = installFetch([body('waiting'), body('ready'), json(404, {}), body('weird'), json(503, {})]);
  assert.equal((await a.status(ID, TOKEN)).state, 'waiting');
  assert.equal(calls[0].url, `https://gone.test/api/request/${ID}/status`);
  assert.equal(calls[0].init.headers['X-Gone-Manage'], TOKEN);
  assert.equal((await a.status(ID, TOKEN)).state, 'ready');
  assert.deepEqual(await a.status(ID, TOKEN), { state: 'gone' });
  await assert.rejects(a.status(ID, TOKEN), (e) => e.retryable === true);
  await assert.rejects(a.status(ID, TOKEN), (e) => e.retryable === true);
});

test('status and cancel never send a malformed token or id', async () => {
  const a = api();
  const calls = installFetch([]);
  assert.deepEqual(await a.status(ID, 'short'), { state: 'gone' });
  assert.equal(await a.cancel(ID, 'short'), 'gone');
  await assert.rejects(a.status('../x', TOKEN), (e) => a.isRequestError(e) && !e.retryable);
  assert.equal(calls.length, 0);
  assert.equal(a.isID(ID), true);
  assert.equal(a.isToken(TOKEN), true);
  assert.equal(a.isRequestError(null), false);
});

test('cancel maps 204, 404 and errors', async () => {
  const a = api();
  const calls = installFetch([json(204, {}), json(404, {}), json(500, {})]);
  assert.equal(await a.cancel(ID, TOKEN), 'cancelled');
  assert.equal(calls[0].init.method, 'POST');
  assert.equal(calls[0].init.keepalive, true);
  assert.equal(calls[0].url, `https://gone.test/api/request/${ID}/revoke`);
  assert.equal(await a.cancel(ID, TOKEN), 'gone');
  await assert.rejects(a.cancel(ID, TOKEN));
});

test('loads once', () => {
  const a = api();
  load('requestApi');
  assert.equal(window.goneRequestApi, a);
});
