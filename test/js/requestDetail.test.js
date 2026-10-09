/* global Buffer, document */
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, waitFor, captureConsole } = require('./harness');
const { fakeIndexedDB } = require('./fakeIdb');
const { fakeResponse, installFetch } = require('./fakes');
const { detailPage } = require('./requestPage');

const ID = 'a'.repeat(32);
const TOKEN = 'T'.repeat(43);
const MODULES = ['crypto', 'cryptoV3', 'util', 'fileMeta', 'envelope', 'icons', 'consumeApi', 'consumeView', 'consumeOpener',
  'requestStore', 'requestApi', 'requestPoll', 'requestDetailView'];
const json = (status, body) => () => Object.assign(fakeResponse({ status }), { json: async () => body });
const status = (state) => json(200, { state, created_at: '2029-12-31T23:00:00Z', expires_at: '2030-01-01T00:00:00Z' });
const flush = () => new Promise((r) => setImmediate(r));

// makeEntry stores a request with a real key pair and returns it.
async function makeEntry(idb, extra) {
  reset('https://gone.test/', { indexedDB: idb });
  load('crypto', 'cryptoV3', 'requestStore');
  const keys = await window.goneCryptoV3.generateKeyPair();
  const entry = Object.assign({
    id: ID, label: 'DB', createdAt: Date.now(), expiresAt: Date.now() + 3600000, manageToken: TOKEN,
    replyLink: `https://gone.test/reply/${ID}#v3:x.y`, publicKey: keys.publicKey, privateKey: keys.privateKey, state: 'waiting'
  }, extra);
  await window.goneRequestStore.put(entry);
  return entry;
}

// replyResponse encrypts message to entry and returns a claim response.
async function replyResponse(entry, message, headers) {
  const enc = await window.goneCryptoV3.encryptV3(message, entry.publicKey);
  const nonce = Buffer.from(enc.nonce).toString('base64url');
  const h = Object.assign({ 'X-Gone-Version': '3', 'X-Gone-Nonce': nonce, 'X-Gone-Claim': 'C'.repeat(43), 'Content-Length': String(enc.ciphertext.length) }, headers);
  return () => fakeResponse({ status: 200, headers: h, chunks: [enc.ciphertext] });
}

async function boot(t, opts) {
  const o = opts || {};
  const idb = fakeIndexedDB();
  const entry = o.noEntry ? null : await makeEntry(idb, o.entry);
  const handlers = typeof o.fetch === 'function' ? await o.fetch(entry) : (o.fetch || []);
  const env = reset(`https://gone.test/request/${o.id || ID}`, { indexedDB: idb });
  const $ = detailPage(env);
  const logs = captureConsole(t);
  const calls = installFetch(handlers);
  if (!o.again) t.mock.timers.enable({ apis: ['setTimeout'] });
  load(...MODULES);
  window.goneUtil = Object.assign({}, window.goneUtil, { sleep: async () => {} });
  if (o.before) o.before();
  load('requestDetail');
  return { env, $, calls, logs, entry, store: window.goneRequestStore };
}

test('waiting shows the facts and the saved link, then polls', async (t) => {
  const b = await boot(t, { fetch: [status('waiting'), status('waiting')] });
  await waitFor(() => !b.$('view-waiting').hidden);
  assert.equal(b.$('waiting-label').textContent, 'DB');
  assert.equal(b.$('waiting-link').value, b.entry.replyLink);
  assert.equal(b.calls[0].init.headers['X-Gone-Manage'], TOKEN);
  assert.equal(document.title, 'Gone · Waiting for a reply');
  await flush();
  b.$('check-again').click();
  await waitFor(() => /Still waiting as of/.test(b.$('waiting-status').textContent));
  assert.equal(b.calls.length, 2);
});

test('a ready reply opens with this browser’s key and is then forgotten', async (t) => {
  const b = await boot(t, { fetch: async (entry) => [status('ready'), await replyResponse(entry, 'hunter2'), json(204, {})] });
  await waitFor(() => !b.$('view-open').hidden);
  assert.ok(b.$('open-expires').getAttribute('datetime'));
  b.$('open-secret').click();
  await waitFor(() => !b.$('view-revealed').hidden);
  assert.equal(b.calls[1].url, `https://gone.test/api/request/${ID}/reply`);
  assert.equal(b.calls[1].init.headers['X-Gone-Manage'], TOKEN);
  assert.equal(b.calls[2].init.method, 'DELETE');
  b.$('show-secret').click();
  assert.equal(b.$('secret-output').textContent, 'hunter2');
  await waitFor(async () => true);
  await flush();
  assert.equal(await b.store.get(ID), undefined);
});

test('a reply that fails to decrypt is reported and nothing is acknowledged', async (t) => {
  const b = await boot(t, { fetch: async (entry) => [status('ready'), await replyResponse(entry, 'x', { 'X-Gone-Version': '2' })] });
  await waitFor(() => !b.$('view-open').hidden);
  b.$('open-secret').click();
  await waitFor(() => !b.$('consume-error').hidden);
  assert.match(b.$('consume-error-text').textContent, /Couldn’t decrypt this reply/);
  assert.equal(b.calls.length, 2);
});

test('a reply already gone when opened forgets the request', async (t) => {
  const b = await boot(t, { fetch: [status('ready'), json(404, {})] });
  await waitFor(() => !b.$('view-open').hidden);
  b.$('open-secret').click();
  await waitFor(() => !b.$('view-gone').hidden);
  await flush();
  assert.equal(await b.store.get(ID), undefined);
});

test('a gone request is forgotten', async (t) => {
  const b = await boot(t, { fetch: [json(404, {})] });
  await waitFor(() => !b.$('view-gone').hidden);
  await flush();
  assert.equal(await b.store.get(ID), undefined);
});

test('a request not saved here, expired, or with a bad id shows missing', async (t) => {
  let b = await boot(t, { noEntry: true });
  await waitFor(() => !b.$('view-missing').hidden);
  assert.equal(b.calls.length, 0);
  b = await boot(t, { entry: { expiresAt: Date.now() - 1 }, again: true });
  await waitFor(() => !b.$('view-missing').hidden);
  b = await boot(t, { id: 'nope', again: true });
  await waitFor(() => !b.$('view-missing').hidden);
});

test('a first-load error offers a retry', async (t) => {
  const b = await boot(t, { fetch: [json(503, {}), status('waiting')] });
  await waitFor(() => !b.$('check-error').hidden);
  assert.match(b.$('check-error-text').textContent, /busy/);
  assert.equal(b.$('check-retry').hidden, false);
  await flush();
  b.$('check-retry').click();
  await waitFor(() => !b.$('view-waiting').hidden);
});

test('a later check error is shown on the waiting view', async (t) => {
  const b = await boot(t, { fetch: [status('waiting'), () => { throw new Error('offline'); }] });
  await waitFor(() => !b.$('view-waiting').hidden);
  await flush();
  b.$('check-again').click();
  await waitFor(() => !b.$('waiting-error').hidden);
  assert.match(b.$('waiting-error-text').textContent, /reach the server/);
});

test('cancel asks first, then deletes and forgets', async (t) => {
  const b = await boot(t, { fetch: [status('waiting'), json(204, {})] });
  await waitFor(() => !b.$('view-waiting').hidden);
  await flush();
  b.$('cancel-now').click();
  assert.equal(b.$('cancel-confirm').hidden, false);
  b.$('cancel-confirm').dispatch('keydown', { key: 'Escape' });
  assert.equal(b.$('cancel-confirm').hidden, true);
  b.$('cancel-now').click();
  b.$('cancel-no').click();
  assert.equal(b.$('cancel-start').hidden, false);
  b.$('cancel-now').click();
  b.$('cancel-yes').click();
  await waitFor(() => !b.$('view-cancelled').hidden);
  assert.equal(b.calls[1].url, `https://gone.test/api/request/${ID}/revoke`);
  assert.equal(await b.store.get(ID), undefined);
});

test('cancel of an already-gone request, and a cancel error', async (t) => {
  let b = await boot(t, { fetch: [status('waiting'), json(404, {})] });
  await waitFor(() => !b.$('view-waiting').hidden);
  await flush();
  b.$('cancel-yes').click();
  await waitFor(() => !b.$('view-gone').hidden);
  b = await boot(t, { fetch: [status('waiting'), json(500, {})], again: true });
  await waitFor(() => !b.$('view-waiting').hidden);
  await flush();
  b.$('cancel-now').click();
  b.$('cancel-yes').click();
  await waitFor(() => !b.$('waiting-error').hidden);
  assert.equal(b.$('cancel-confirm').hidden, true);
});

test('copy puts the request link on the clipboard', async (t) => {
  const b = await boot(t, { fetch: [status('waiting')] });
  await waitFor(() => !b.$('view-waiting').hidden);
  b.$('copy-waiting-link').click();
  await waitFor(() => b.env.clipboard.text === b.entry.replyLink);
  b.env.clipboard.fail = true;
  b.$('copy-waiting-link').click();
  await waitFor(() => b.$('waiting-link').selected === true);
});

test('an unexpected error is logged with a generic message', async (t) => {
  const b = await boot(t, { fetch: [json(200, { state: 'waiting', created_at: 'x', expires_at: 'y' })] });
  await waitFor(() => !b.$('check-error').hidden);
  assert.match(b.$('check-error-text').textContent, /server had a problem/);
});

test('the page does nothing without its views', (t) => {
  reset('https://gone.test/request/' + ID);
  captureConsole(t);
  load(...MODULES, 'requestDetail');
  assert.equal(document.title, '');
});

test('an insecure page shows a ready reply but cannot open it', async (t) => {
  const idb = fakeIndexedDB();
  await makeEntry(idb);
  const real = Object.getOwnPropertyDescriptor(globalThis, 'crypto');
  const env = reset(`https://gone.test/request/${ID}`, { indexedDB: idb });
  const $ = detailPage(env);
  captureConsole(t);
  installFetch([status('ready')]);
  t.mock.timers.enable({ apis: ['setTimeout'] });
  load(...MODULES);
  Object.defineProperty(globalThis, 'crypto', { configurable: true, value: {} });
  t.after(() => Object.defineProperty(globalThis, 'crypto', real));
  load('requestDetail');
  await waitFor(() => !$('view-open').hidden);
  assert.match($('consume-error-text').textContent, /secure \(HTTPS\) pages/);
  assert.equal($('open-secret').getAttribute('aria-disabled'), 'true');
});

test('an unreadable store and unexpected errors fall back safely', async (t) => {
  const b = await boot(t, { noEntry: true });
  await waitFor(() => !b.$('view-missing').hidden);
  const idb = fakeIndexedDB({ failOpen: true });
  const env = reset(`https://gone.test/request/${ID}`, { indexedDB: idb });
  const $ = detailPage(env);
  installFetch([]);
  load(...MODULES, 'requestDetail');
  await waitFor(() => !$('view-missing').hidden);
});

test('a non-request error is logged and shown generically', async (t) => {
  const b = await boot(t, {
    before: () => {
      window.goneRequestApi = Object.assign({}, window.goneRequestApi, { status: async () => { throw new Error('boom'); } });
    }
  });
  await waitFor(() => !b.$('check-error').hidden);
  assert.equal(b.$('check-error-text').textContent, 'Something went wrong. Try again.');
  assert.equal(b.$('check-retry').hidden, false);
  assert.match(b.logs.error[0], /request error/);
});
