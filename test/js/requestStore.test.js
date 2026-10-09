'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');
const { fakeIndexedDB } = require('./fakeIdb');

function boot(idbOpts, extra) {
  const idb = idbOpts === null ? undefined : fakeIndexedDB(idbOpts);
  reset('https://gone.test/request', Object.assign({ indexedDB: idb }, extra));
  load('requestStore');
  return window.goneRequestStore;
}

test('put, get, list and remove entries', async () => {
  const s = boot({});
  assert.equal(await s.available(), true);
  await s.put({ id: 'a', createdAt: 1, expiresAt: 100 });
  await s.put({ id: 'b', createdAt: 2, expiresAt: 100 });
  assert.equal((await s.get('a')).createdAt, 1);
  assert.deepEqual((await s.list(50)).map((e) => e.id), ['b', 'a']);
  await s.remove('a');
  assert.equal(await s.get('a'), undefined);
});

test('list prunes expired entries and hides the probe', async () => {
  const s = boot({});
  await s.available();
  await s.put({ id: 'old', createdAt: 1, expiresAt: 10 });
  await s.put({ id: 'new', createdAt: 2, expiresAt: Date.now() + 60000 });
  assert.deepEqual((await s.list(20)).map((e) => e.id), ['new']);
  assert.equal(await s.get('old'), undefined);
  assert.deepEqual((await s.list()).map((e) => e.id), ['new']);
});

test('available is false without IndexedDB, when open fails, or when writes fail', async () => {
  assert.equal(await boot(null).available(), false);
  assert.equal(await boot({ failOpen: true }).available(), false);
  assert.equal(await boot({ blocked: true }).available(), false);
  assert.equal(await boot({ failWrites: true }).available(), false);
});

test('a failed open is retried on the next call', async () => {
  const s = boot({ failOpen: true });
  await assert.rejects(s.get('x'));
  await assert.rejects(s.get('x'));
});

test('persist asks for persistent storage and tolerates every failure', async () => {
  const s = boot({});
  let asked = 0;
  globalThis.navigator.storage = { persist: async () => { asked++; throw new Error('no'); } };
  s.persist();
  globalThis.navigator.storage = undefined;
  s.persist();
  Object.defineProperty(globalThis.navigator, 'storage', { get() { throw new Error('blocked'); }, configurable: true });
  s.persist();
  delete globalThis.navigator.storage;
  await new Promise((r) => setImmediate(r));
  assert.equal(asked, 1);
});

test('loads once', () => {
  const s = boot({});
  load('requestStore');
  assert.equal(window.goneRequestStore, s);
});
