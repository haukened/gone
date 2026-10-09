/* global document */
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, waitFor } = require('./harness');

function poll(t) {
  reset('https://gone.test/request');
  load('requestPoll');
  t.mock.timers.enable({ apis: ['setTimeout'] });
  return window.goneRequestPoll;
}

const flush = () => new Promise((r) => setImmediate(r));

test('checks at once, then every interval until stop', async (t) => {
  const p = poll(t);
  const answers = ['continue', 'continue', 'stop'];
  let n = 0;
  const handle = p.create(async () => { n++; return answers.shift(); });
  await handle.start();
  assert.equal(n, 1);
  t.mock.timers.tick(p.INTERVAL_MS);
  await waitFor(() => n === 2);
  await flush();
  t.mock.timers.tick(p.INTERVAL_MS);
  await waitFor(() => n === 3);
  t.mock.timers.tick(p.INTERVAL_MS * 3);
  await flush();
  assert.equal(n, 3);
});

test('backs off after an error, honouring a longer Retry-After', async (t) => {
  const p = poll(t);
  let n = 0;
  const errors = [Object.assign(new Error('busy'), { retryAfter: 120 }), new Error('down')];
  const handle = p.create(async () => { n++; if (errors.length) throw errors.shift(); return 'stop'; });
  await handle.start();
  t.mock.timers.tick(p.BACKOFF_MS);
  await flush();
  assert.equal(n, 1);
  t.mock.timers.tick(120000 - p.BACKOFF_MS);
  await waitFor(() => n === 2);
  await flush();
  t.mock.timers.tick(p.BACKOFF_MS);
  await waitFor(() => n === 3);
});

test('pauses while hidden and checks when shown again', async (t) => {
  const p = poll(t);
  let n = 0;
  const handle = p.create(async () => { n++; return 'continue'; });
  document.visibilityState = 'hidden';
  await handle.start();
  assert.equal(n, 0);
  document.dispatch('visibilitychange');
  assert.equal(n, 0);
  document.visibilityState = 'visible';
  document.dispatch('visibilitychange');
  await waitFor(() => n === 1);
  document.dispatch('visibilitychange');
  await flush();
  assert.equal(n, 1);
  handle.stop();
  t.mock.timers.tick(p.INTERVAL_MS);
  await flush();
  assert.equal(n, 1);
});

test('overlapping checks are ignored', async (t) => {
  const p = poll(t);
  let n = 0;
  let release;
  const handle = p.create(() => { n++; return new Promise((r) => { release = r; }); });
  const first = handle.start();
  await handle.now();
  assert.equal(n, 1);
  release('stop');
  await first;
  await handle.now();
  assert.equal(n, 1);
});

test('loads once', (t) => {
  const p = poll(t);
  load('requestPoll');
  assert.equal(window.goneRequestPoll, p);
});
