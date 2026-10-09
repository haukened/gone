/* global document */
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, waitFor, captureConsole } = require('./harness');
const { fakeIndexedDB } = require('./fakeIdb');
const { fakeResponse, installFetch } = require('./fakes');
const { requestPage } = require('./requestPage');

const ID = 'a'.repeat(32);
const TOKEN = 'T'.repeat(43);
const FILL = 'F'.repeat(43);
const MODULES = ['crypto', 'cryptoV3', 'util', 'requestStore', 'requestApi', 'requestPoll', 'requestList'];
const json = (status, body) => () => Object.assign(fakeResponse({ status }), { json: async () => body });
const created = (id) => json(201, { id: id || ID, expires_at: '2030-01-01T00:00:00Z', manage_token: TOKEN, fill_token: FILL });
const status = (state, exp) => json(200, { state, created_at: '2029-12-31T23:00:00Z', expires_at: exp || '2030-01-01T00:00:00Z' });

function boot(t, opts) {
  const o = opts || {};
  const idb = o.idb || fakeIndexedDB(o.idbOpts);
  const env = reset('https://gone.test/request', { indexedDB: o.noIdb ? undefined : idb });
  const $ = requestPage(env, o.ttl);
  const logs = captureConsole(t);
  const calls = installFetch(o.fetch || []);
  t.mock.timers.enable({ apis: ['setTimeout'] });
  load(...MODULES);
  return { env, $, calls, logs, idb, start: () => load('request') };
}

const submit = (b) => b.$('create-request').dispatch('submit');

async function seed(idb, entries) {
  reset('https://gone.test/request', { indexedDB: idb });
  load('requestStore');
  for (const e of entries) await window.goneRequestStore.put(e);
}

test('creating a request saves the key here and shows the reply link', async (t) => {
  const b = boot(t, { fetch: [created(), status('waiting'), status('ready')], ttl: '30m' });
  b.start();
  await waitFor(() => b.$('create-request-btn').getAttribute('aria-disabled') === 'false' || b.calls.length === 0);
  b.$('request-label').value = '  Staging DB  ';
  submit(b);
  await waitFor(() => !b.$('view-created').hidden);
  assert.equal(b.calls[0].init.headers['X-Gone-TTL'], '30m');
  const link = b.$('reply-link').value;
  assert.match(link, new RegExp(`^https://gone\\.test/reply/${ID}#v3:[A-Za-z0-9_-]{87}\\.${FILL}$`));
  assert.equal(b.$('created-open').href, `/request/${ID}`);
  assert.equal(document.title, 'Gone · Request link ready');
  const entry = await window.goneRequestStore.get(ID);
  assert.equal(entry.label, 'Staging DB');
  assert.equal(entry.manageToken, TOKEN);
  assert.equal(entry.privateKey.extractable, false);
  assert.equal(entry.state, 'waiting');
  await waitFor(() => b.calls.length === 2);
  assert.equal(b.calls[1].url, `https://gone.test/api/request/${ID}/status`);
  document.visibilityState = 'hidden';
  await new Promise((r) => setImmediate(r));
  t.mock.timers.tick(20000);
  document.visibilityState = 'visible';
  document.dispatch('visibilitychange');
  await waitFor(() => b.$('created-state').dataset.state === 'ready');
  assert.equal(b.$('created-open').textContent, 'Open the reply');
  assert.ok(b.$('created-step-replied').classList.contains('is-now'));
  assert.equal((await window.goneRequestStore.get(ID)).state, 'ready');
});

test('a reply that arrives while the tab is hidden marks the title', async (t) => {
  const hideThenReady = () => { document.visibilityState = 'hidden'; return status('ready')(); };
  const b = boot(t, { fetch: [created(), hideThenReady] });
  b.start();
  await new Promise((r) => setImmediate(r));
  submit(b);
  await waitFor(() => b.$('created-state').dataset.state === 'ready');
  assert.equal(document.title, '\u25cf Reply ready \u00b7 Gone \u00b7 Request link ready');
  document.visibilityState = 'visible';
  document.dispatch('visibilitychange');
  assert.equal(document.title, 'Gone \u00b7 Request link ready');
});

test('a created request that is gone says so', async (t) => {
  const b = boot(t, { fetch: [created(), json(404, {})] });
  b.start();
  await new Promise((r) => setImmediate(r));
  submit(b);
  await waitFor(() => b.$('created-state').dataset.state === 'gone');
  assert.equal(b.$('created-status').textContent, 'This request is gone.');
  assert.equal(await window.goneRequestStore.get(ID), undefined);
});

test('saved requests are listed and polled until none are waiting', async (t) => {
  const idb = fakeIndexedDB();
  const now = Date.now();
  await seed(idb, [
    { id: ID, label: 'A', createdAt: now - 1000, expiresAt: now + 3600000, manageToken: TOKEN, state: 'waiting' },
    { id: 'b'.repeat(32), label: 'B', createdAt: now - 2000, expiresAt: now + 3600000, manageToken: TOKEN, state: 'ready' },
    { id: 'c'.repeat(32), label: 'C', createdAt: now - 3000, expiresAt: now + 3600000, manageToken: TOKEN, state: 'waiting' }
  ]);
  const b = boot(t, { idb, fetch: [status('ready', '2030-01-01T01:00:00Z'), json(404, {})] });
  b.start();
  await waitFor(() => b.$('request-list-status').textContent === 'A reply arrived.');
  const states = b.$('request-list').children.map((li) => li.children[0].children[2].dataset.state);
  assert.deepEqual(states, ['ready', 'ready']);
  assert.equal(b.calls.length, 2);
  t.mock.timers.tick(60000);
  await new Promise((r) => setImmediate(r));
  assert.equal(b.calls.length, 2);
});

test('two replies arriving together are counted', async (t) => {
  const idb = fakeIndexedDB();
  const now = Date.now();
  await seed(idb, [
    { id: ID, createdAt: now, expiresAt: now + 3600000, manageToken: TOKEN, state: 'waiting' },
    { id: 'b'.repeat(32), createdAt: now, expiresAt: now + 3600000, manageToken: TOKEN, state: 'waiting' }
  ]);
  const b = boot(t, { idb, fetch: [status('ready'), status('ready')] });
  b.start();
  await waitFor(() => b.$('request-list-status').textContent === '2 replies arrived.');
});

test('a browser that cannot keep keys turns requests off', async (t) => {
  const b = boot(t, { idbOpts: { failWrites: true } });
  b.start();
  await waitFor(() => !b.$('storage-alert').hidden);
  assert.equal(b.$('create-request-btn').getAttribute('aria-disabled'), 'true');
  submit(b);
  assert.equal(b.calls.length, 0);
});

test('an insecure page turns requests off', async (t) => {
  const real = Object.getOwnPropertyDescriptor(globalThis, 'crypto');
  Object.defineProperty(globalThis, 'crypto', { configurable: true, value: {} });
  t.after(() => Object.defineProperty(globalThis, 'crypto', real));
  const b = boot(t);
  b.start();
  assert.match(b.$('request-hint').textContent, /isn’t HTTPS/);
  assert.equal(b.$('create-request-btn').getAttribute('aria-disabled'), 'true');
});

test('a key that cannot be saved cancels the request', async (t) => {
  const idb = fakeIndexedDB();
  const b = boot(t, { idb, fetch: [created(), json(204, {})] });
  b.start();
  await new Promise((r) => setImmediate(r));
  await waitFor(() => b.$('create-request-btn').getAttribute('aria-disabled') !== 'true');
  idb.dbs.get('gone').opts.failWrites = true;
  submit(b);
  await waitFor(() => !b.$('request-error').hidden);
  assert.match(b.$('request-error-text').textContent, /couldn’t save/);
  assert.equal(b.calls[1].url, `https://gone.test/api/request/${ID}/revoke`);
  assert.match(b.logs.error[0], /could not save request/);
});

test('server and unexpected errors are shown; busy ignores resubmits', async (t) => {
  const b = boot(t, { fetch: [json(503, {}), () => { throw new Error('offline'); }] });
  b.start();
  await new Promise((r) => setImmediate(r));
  submit(b);
  submit(b);
  await waitFor(() => !b.$('request-error').hidden);
  assert.match(b.$('request-error-text').textContent, /busy/);
  assert.equal(b.calls.length, 1);
  assert.equal(b.$('create-request-btn').querySelector('span').textContent, 'Create request link');
});

test('an unexpected failure shows a generic message', async (t) => {
  const b = boot(t);
  globalThis.goneCryptoV3 = Object.assign({}, window.goneCryptoV3, { generateKeyPair: async () => { throw new Error('boom'); } });
  b.start();
  await new Promise((r) => setImmediate(r));
  submit(b);
  await waitFor(() => !b.$('request-error').hidden);
  assert.match(b.$('request-error-text').textContent, /Something went wrong/);
  assert.match(b.logs.error[0], /request failed/);
});

test('copy puts the link on the clipboard', async (t) => {
  const b = boot(t, { fetch: [created(), status('waiting')] });
  b.start();
  await new Promise((r) => setImmediate(r));
  submit(b);
  await waitFor(() => !b.$('view-created').hidden);
  b.$('copy-reply').click();
  await waitFor(() => b.$('copy-status').textContent !== '');
  assert.equal(b.env.clipboard.text, b.$('reply-link').value);
  b.env.clipboard.fail = true;
  b.$('copy-reply').click();
  await waitFor(() => b.$('reply-link').selected === true);
});

test('the page does nothing without its form', (t) => {
  reset('https://gone.test/request');
  captureConsole(t);
  load(...MODULES, 'request');
  assert.equal(document.title, '');
});

test('making a request stops the list from polling', async (t) => {
  const idb = fakeIndexedDB();
  const now = Date.now();
  await seed(idb, [{ id: 'b'.repeat(32), createdAt: now, expiresAt: now + 3600000, manageToken: TOKEN, state: 'waiting' }]);
  const b = boot(t, { idb, fetch: [status('waiting'), created(), status('waiting'), status('waiting'), status('waiting')] });
  b.start();
  await waitFor(() => b.calls.length === 1);
  await new Promise((r) => setImmediate(r));
  submit(b);
  await waitFor(() => b.calls.length === 3);
  await new Promise((r) => setImmediate(r));
  t.mock.timers.tick(20000);
  await waitFor(() => b.calls.length >= 4);
  await new Promise((r) => setImmediate(r));
  // Only the new request is polled; the hidden list is not.
  assert.equal(b.calls.length, 4);
  assert.equal(b.calls[3].url, `https://gone.test/api/request/${ID}/status`);
});

test('a list that cannot be read renders empty', async (t) => {
  const b = boot(t);
  window.goneRequestStore = Object.assign({}, window.goneRequestStore, { list: async () => { throw new Error('gone'); } });
  b.start();
  await waitFor(() => b.$('request-empty').hidden === false && b.$('request-list').hidden === true);
  await new Promise((r) => setImmediate(r));
  assert.equal(b.calls.length, 0);
});
