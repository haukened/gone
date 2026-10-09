/* global Buffer, document */
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h, waitFor, captureConsole } = require('./harness');
const { fakeResponse, installFetch, FakeXHR } = require('./fakes');

const ID = 'a'.repeat(32);
const FILL = 'F'.repeat(43);
const MODULES = ['util', 'crypto', 'cryptoV3', 'fileMeta', 'envelope', 'icons', 'submitFiles', 'submitMeter', 'submitUpload', 'reply', 'submit'];
const json = (status, body) => () => Object.assign(fakeResponse({ status }), { json: async () => body });
const b64u = (b) => Buffer.from(b).toString('base64url');

function replyPage(env) {
  const view = (id, hidden, kids, heading) => h('div', { id, hidden }, [h('h1', { id: heading }), ...kids]);
  const form = h('form', { id: 'create-secret', dataset: { maxBytes: '100', overhead: '65' }, elements: { namedItem: () => null } }, [
    h('textarea', { id: 'secret' }),
    h('div', { id: 'drop-zone' }, [h('input', { id: 'secret-files' })]),
    h('ul', { id: 'file-list', hidden: true }),
    h('div', { id: 'size-box' }), h('progress', { id: 'size-meter' }), h('span', { id: 'size-label' }),
    h('div', { id: 'size-warning', hidden: true }, [h('p', { id: 'size-warning-text' })]),
    h('button', { type: 'submit' }, [h('span', { textContent: 'Send securely' })]),
    h('progress', { id: 'upload-progress', hidden: true }),
    h('span', { id: 'submit-hint' }, [h('time', { id: 'reply-expires' })]),
    h('div', { id: 'submit-error', hidden: true }, [h('p', { id: 'submit-error-content' })])
  ]);
  env.document.body.append(
    view('view-check', false, [
      h('span', { id: 'check-status' }),
      h('div', { id: 'check-error', hidden: true }, [h('p', { id: 'check-error-text' })]),
      h('button', { id: 'check-retry', hidden: true })
    ], 'check-heading'),
    view('compose', true, [form], 'compose-heading'),
    view('view-sent', true, [], 'sent-heading'),
    view('view-gone', true, [], 'gone-heading')
  );
  return (id) => env.document.getElementById(id);
}

async function keys() {
  reset();
  load('crypto', 'cryptoV3');
  return window.goneCryptoV3.generateKeyPair();
}

async function boot(t, opts) {
  const o = opts || {};
  const kp = o.kp || await keys();
  const hash = o.hash !== undefined ? o.hash : `#v3:${b64u(o.pub || kp.publicKey)}.${FILL}`;
  const env = reset(`https://gone.test/reply/${o.id || ID}${hash}`);
  const $ = replyPage(env);
  const logs = captureConsole(t);
  const calls = installFetch(o.fetch || [json(200, { state: 'open', expires_at: '2030-01-01T00:00:00Z' })]);
  FakeXHR.reset();
  globalThis.XMLHttpRequest = FakeXHR;
  load(...MODULES);
  return { env, $, calls, logs, kp };
}

const type = (b, text) => { b.$('secret').value = text; b.$('secret').dispatch('input'); };
const send = (b) => b.$('create-secret').dispatch('submit');

test('an open request shows the form, and the reply is sealed to the requester', async (t) => {
  const b = await boot(t);
  await waitFor(() => !b.$('compose').hidden);
  assert.equal(b.calls[0].url, `https://gone.test/api/request/${ID}`);
  assert.equal(b.calls[0].init.headers['X-Gone-Fill'], FILL);
  assert.equal(b.$('reply-expires').getAttribute('datetime'), '2030-01-01T00:00:00.000Z');
  type(b, 'hunter2');
  assert.match(b.$('size-label').textContent, /^88 B/);
  let sent;
  FakeXHR.onSend = (x) => { sent = x; x.respond(201, { expires_at: '2030-01-01T00:00:00Z' }); };
  send(b);
  await waitFor(() => !b.$('view-sent').hidden);
  assert.equal(sent.method, 'PUT');
  assert.equal(sent.url, `/api/request/${ID}/reply`);
  assert.equal(sent.headers['X-Gone-Version'], '3');
  assert.equal(sent.headers['X-Gone-Fill'], FILL);
  assert.equal(document.title, 'Gone · Sent');
  assert.equal(b.$('secret').value, '');
  assert.equal(sent.body.every((x) => x === 0), true, 'ciphertext wiped after send');
});

test('the sealed reply opens with the requester’s private key', async (t) => {
  const b = await boot(t);
  await waitFor(() => !b.$('compose').hidden);
  type(b, 'for your eyes');
  let copy;
  FakeXHR.onSend = (x) => { copy = { body: x.body.slice(), nonce: x.headers['X-Gone-Nonce'] }; x.respond(201, {}); };
  send(b);
  await waitFor(() => !b.$('view-sent').hidden);
  const nonce = new Uint8Array(Buffer.from(copy.nonce, 'base64url'));
  const pt = await window.goneCryptoV3.decryptV3(copy.body, nonce, b.kp.privateKey, b.kp.publicKey);
  assert.equal(new TextDecoder().decode(pt), 'for your eyes');
});

test('a request answered meanwhile switches to the gone view', async (t) => {
  const b = await boot(t);
  await waitFor(() => !b.$('compose').hidden);
  type(b, 'x');
  FakeXHR.onSend = (x) => x.respond(404);
  send(b);
  await waitFor(() => !b.$('view-gone').hidden);
});

test('other upload failures stay on the form with a message', async (t) => {
  const b = await boot(t);
  await waitFor(() => !b.$('compose').hidden);
  type(b, 'x');
  FakeXHR.onSend = (x) => x.respond(413);
  send(b);
  await waitFor(() => !b.$('submit-error').hidden);
  assert.match(b.$('submit-error-content').textContent, /too large/i);
});

test('a public key that is not on the curve cannot be answered', async (t) => {
  const bad = new Uint8Array(65);
  bad[0] = 4;
  const b = await boot(t, { pub: bad });
  await waitFor(() => !b.$('compose').hidden);
  type(b, 'x');
  send(b);
  await waitFor(() => !b.$('submit-error').hidden);
  assert.match(b.$('submit-error-content').textContent, /link is damaged/);
  assert.equal(FakeXHR.instances.length, 0);
});

test('a closed request shows the gone view before anything is typed', async (t) => {
  const b = await boot(t, { fetch: [json(404, {})] });
  await waitFor(() => !b.$('view-gone').hidden);
  assert.equal(b.$('compose').hidden, true);
});

test('check errors offer a retry where it can help', async (t) => {
  const b = await boot(t, { fetch: [json(429, {}), () => { throw new Error('offline'); }, json(500, {}), json(200, {})] });
  await waitFor(() => !b.$('check-error').hidden);
  assert.match(b.$('check-error-text').textContent, /Too many requests/);
  assert.equal(b.$('check-retry').hidden, false);
  b.$('check-retry').click();
  await waitFor(() => /reach the server/.test(b.$('check-error-text').textContent));
  b.$('check-retry').click();
  await waitFor(() => /server had a problem/.test(b.$('check-error-text').textContent));
  b.$('check-retry').click();
  await waitFor(() => !b.$('compose').hidden);
  assert.equal(b.$('reply-expires').textContent, '');
});

test('a rejected link says so without retry', async (t) => {
  const b = await boot(t, { fetch: [json(400, {})] });
  await waitFor(() => !b.$('check-error').hidden);
  assert.equal(b.$('check-retry').hidden, true);
});

test('malformed links are rejected before any request', async (t) => {
  const kp = await keys();
  const cases = [
    { hash: '' },
    { hash: `#v3:${b64u(kp.publicKey)}` },
    { id: 'nope' },
    { hash: `#v4:${b64u(kp.publicKey)}.${FILL}`, message: /newer version/ }
  ];
  for (const c of cases) {
    const b = await boot(t, Object.assign({ kp }, c));
    assert.equal(b.$('check-error').hidden, false, JSON.stringify(c));
    assert.match(b.$('check-error-text').textContent, c.message || /isn’t complete/);
    assert.equal(b.calls.length, 0);
    assert.equal(window.goneSubmitTarget, undefined);
  }
});

test('the script does nothing on other pages or twice', (t) => {
  reset('https://gone.test/');
  captureConsole(t);
  load(...MODULES);
  assert.equal(window.goneSubmitTarget, undefined);
  window.goneSubmitTarget = { mine: true };
  load('reply');
  assert.deepEqual(window.goneSubmitTarget, { mine: true });
});
