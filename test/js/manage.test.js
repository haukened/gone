'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, captureConsole, waitFor } = require('./harness');
const { installFetch } = require('./fakes');
const { managePage } = require('./managePage');

const ID = '0123456789abcdef0123456789abcdef';
const TOKEN = 'B'.repeat(43);
const URL_OK = `https://gone.test/manage/${ID}#${TOKEN}`;
const PENDING = { state: 'pending', created_at: '2030-01-01T00:00:00Z', expires_at: '2030-01-02T00:00:00Z' };

const ok = (body) => () => ({ status: 200, json: async () => body });
const code = (status) => () => ({ status, json: async () => ({}) });

// deferred returns a fetch handler that waits for release(handler).
function deferred() {
  let release;
  const gate = new Promise((r) => { release = r; });
  return { handler: () => gate.then((h) => h()), release };
}

// boot builds the manage page at url, queues fetch handlers, and runs
// manage.js. opts.api patches goneManageApi before manage.js loads.
function boot(t, url, handlers, opts) {
  const o = opts || {};
  const env = reset(url);
  const $ = managePage(env, o);
  const logs = captureConsole(t);
  const calls = installFetch(handlers || []);
  load('util', 'manageApi', 'manageView');
  if (o.api) window.goneManageApi = Object.assign({}, window.goneManageApi, o.api);
  load('manage');
  const visible = () => ['check', 'pending', 'deleted', 'gone'].filter((v) => !$(`view-${v}`).hidden);
  return { env, $, logs, calls, visible };
}

// booted boots to the pending view with one successful status check.
async function booted(t, handlers) {
  const b = boot(t, URL_OK, [ok(PENDING), ...handlers]);
  await waitFor(() => b.visible()[0] === 'pending');
  return b;
}

test('does nothing without dependencies or the manage page', (t) => {
  reset(URL_OK);
  const calls = installFetch([]);
  load('manage');
  load('util', 'manageApi', 'manageView');
  load('manage');
  assert.equal(calls.length, 0);
  const b = boot(t, URL_OK, [], { skip: ['view-pending'] });
  assert.equal(b.calls.length, 0);
});

test('malformed links are rejected before any request', async (t) => {
  const bad = [
    `https://gone.test/manage/${ID}`,
    `https://gone.test/manage/${ID}#${TOKEN}x`,
    `https://gone.test/manage/${ID.toUpperCase()}#${TOKEN}`,
    `https://gone.test/manage/#${TOKEN}`
  ];
  for (const url of bad) {
    const b = boot(t, url);
    assert.equal(b.calls.length, 0, url);
    assert.match(b.$('check-error-text').textContent, /isn\u2019t valid/, url);
    assert.equal(b.$('check-retry').hidden, true, url);
  }
});

test('preview modes render without network', (t) => {
  const cases = [['pending', 'pending'], ['deleted', 'deleted'], ['gone', 'gone'], ['error', 'check']];
  for (const [mode, shown] of cases) {
    const b = boot(t, `https://gone.test/manage/x?preview=${mode}`);
    assert.deepEqual(b.visible(), [shown], mode);
    assert.equal(b.calls.length, 0, mode);
  }
  const b = boot(t, 'https://gone.test/manage/x?preview=pending');
  assert.ok(b.$('manage-created').textContent.length > 0);
  assert.match(boot(t, 'https://gone.test/manage/x?preview=error').$('check-error-text').textContent, /isn\u2019t valid/);
  const unknown = boot(t, 'https://gone.test/manage/x?preview=bogus');
  assert.match(unknown.$('check-error-text').textContent, /isn\u2019t valid/);
});

test('first load shows pending with the token only in a header', async (t) => {
  const b = await booted(t, []);
  assert.equal(b.calls.length, 1);
  assert.equal(b.calls[0].url, `https://gone.test/api/secret/${ID}/status`);
  assert.equal(b.calls[0].init.headers['X-Gone-Manage'], TOKEN);
  assert.equal(b.$('manage-created').getAttribute('datetime'), '2030-01-01T00:00:00.000Z');
  assert.equal(b.env.document.activeElement, b.$('pending-heading'));
});

test('first load of a gone secret shows the gone view', async (t) => {
  const b = boot(t, URL_OK, [code(404)]);
  await waitFor(() => b.visible()[0] === 'gone');
});

test('a retryable first-load error offers Try again', async (t) => {
  const b = boot(t, URL_OK, [code(500), ok(PENDING)]);
  await waitFor(() => !b.$('check-error').hidden);
  assert.match(b.$('check-error-text').textContent, /server had a problem/);
  assert.equal(b.$('check-retry').hidden, false);
  b.$('check-retry').click();
  await waitFor(() => b.visible()[0] === 'pending');
});

test('a 400 on first load is final', async (t) => {
  const b = boot(t, URL_OK, [code(400)]);
  await waitFor(() => !b.$('check-error').hidden);
  assert.match(b.$('check-error-text').textContent, /isn\u2019t valid/);
  assert.equal(b.$('check-retry').hidden, true);
});

test('unexpected errors are logged and shown generically', async (t) => {
  const b = boot(t, URL_OK, [], { api: { status: async () => { throw new TypeError('kaboom'); } } });
  await waitFor(() => !b.$('check-error').hidden);
  assert.equal(b.$('check-error-text').textContent, 'Something went wrong. Try again.');
  assert.equal(b.$('check-retry').hidden, false);
  assert.match(b.logs.error[0], /\[gone\] manage error TypeError: kaboom/);
});

test('Check again refreshes in place and announces the result', async (t) => {
  const b = await booted(t, [ok(PENDING)]);
  b.$('check-again').focus();
  b.$('check-again').click();
  assert.equal(b.$('check-again').disabled, true);
  await waitFor(() => /Still waiting/.test(b.$('pending-status').textContent));
  assert.equal(b.$('check-again').disabled, false);
  assert.equal(b.env.document.activeElement, b.$('check-again'));
});

test('Check again errors stay on the pending view', async (t) => {
  const b = await booted(t, [code(429), ok(PENDING)]);
  b.$('check-again').click();
  await waitFor(() => !b.$('pending-error').hidden);
  assert.match(b.$('pending-error-text').textContent, /Too many requests/);
  assert.deepEqual(b.visible(), ['pending']);
  b.$('check-again').click();
  await waitFor(() => b.$('pending-error').hidden);
});

test('Check again on an opened secret shows gone', async (t) => {
  const b = await booted(t, [code(404)]);
  b.$('check-again').click();
  await waitFor(() => b.visible()[0] === 'gone');
});

test('Delete asks first; Keep it and Escape back out', async (t) => {
  const b = await booted(t, []);
  b.$('delete-now').click();
  assert.equal(b.$('delete-confirm').hidden, false);
  assert.equal(b.env.document.activeElement, b.$('delete-confirm-text'));
  b.$('delete-no').click();
  assert.equal(b.$('delete-confirm').hidden, true);
  assert.equal(b.env.document.activeElement, b.$('delete-now'));
  b.$('delete-now').click();
  b.$('delete-confirm').dispatch('keydown', { key: 'Escape' });
  assert.equal(b.$('delete-confirm').hidden, true);
  assert.equal(b.calls.length, 1);
});

test('confirming deletes the secret', async (t) => {
  const b = await booted(t, [code(204)]);
  b.$('delete-now').click();
  b.$('delete-yes').click();
  assert.equal(b.$('delete-yes').disabled, true);
  await waitFor(() => b.visible()[0] === 'deleted');
  assert.equal(b.calls[1].url, `https://gone.test/api/secret/${ID}/revoke`);
  assert.equal(b.calls[1].init.method, 'POST');
  assert.equal(b.env.document.activeElement, b.$('deleted-heading'));
});

test('deleting an already-gone secret shows gone', async (t) => {
  const b = await booted(t, [code(404)]);
  b.$('delete-now').click();
  b.$('delete-yes').click();
  await waitFor(() => b.visible()[0] === 'gone');
});

test('a failed delete closes the confirm and reports the error', async (t) => {
  const b = await booted(t, [() => { throw new TypeError('offline'); }]);
  b.$('delete-now').click();
  b.$('delete-yes').click();
  await waitFor(() => !b.$('pending-error').hidden);
  assert.match(b.$('pending-error-text').textContent, /Couldn\u2019t reach/);
  assert.equal(b.$('delete-confirm').hidden, true);
  assert.equal(b.$('delete-yes').disabled, false);
  assert.deepEqual(b.visible(), ['pending']);
});

test('busy guard: no overlapping requests or confirm toggles', async (t) => {
  const gate = deferred();
  const b = await booted(t, [gate.handler]);
  b.$('delete-now').click();
  b.$('delete-yes').click();
  b.$('delete-yes').click();
  b.$('check-again').click();
  b.$('delete-no').click();
  b.$('delete-now').click();
  assert.equal(b.$('delete-confirm').hidden, false);
  gate.release(code(204));
  await waitFor(() => b.visible()[0] === 'deleted');
  assert.equal(b.calls.length, 2);
});

test('busy guard: a running check blocks a second check and delete', async (t) => {
  const gate = deferred();
  const b = await booted(t, [gate.handler]);
  b.$('check-again').click();
  b.$('check-again').click();
  b.$('delete-now').click();
  b.$('delete-yes').click();
  assert.equal(b.$('delete-confirm').hidden, true);
  gate.release(ok(PENDING));
  await waitFor(() => !b.$('check-again').disabled);
  assert.equal(b.calls.length, 2);
});
