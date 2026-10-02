'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h, captureConsole, waitFor } = require('./harness');
const { FakeXHR } = require('./fakes');

const MODULES = ['util', 'crypto', 'fileMeta', 'envelope', 'submitFiles', 'submitMeter', 'submitUpload', 'submitResult'];

// page builds the create form. opts.skip omits optional ids; opts.maxBytes
// sets data-max-bytes; opts.noLabel drops the button's <span>.
function page(env, opts) {
  const o = opts || {};
  const skip = new Set(o.skip || []);
  const add = (id, node) => (skip.has(id) ? [] : [node]);
  const label = o.noLabel ? [] : [h('span', { textContent: 'Encrypt' })];
  const form = h('form', { id: 'create-secret', dataset: o.maxBytes === undefined ? { maxBytes: '1000' } : (o.maxBytes === null ? {} : { maxBytes: o.maxBytes }) }, [
    ...add('secret', h('textarea', { id: 'secret' })),
    ...add('drop-zone', h('div', { id: 'drop-zone' }, add('secret-files', h('input', { id: 'secret-files' })))),
    ...add('file-list', h('ul', { id: 'file-list', hidden: true })),
    ...add('size-meter', h('progress', { id: 'size-meter' })),
    ...add('size-label', h('span', { id: 'size-label' })),
    ...add('size-warning', h('div', { id: 'size-warning', hidden: true })),
    ...add('upload-progress', h('progress', { id: 'upload-progress', hidden: true })),
    ...add('ttl', h('select', { id: 'ttl', value: '3600' })),
    ...add('button', h('button', { type: 'submit' }, label))
  ]);
  const card = h(skip.has('card') ? 'div' : 'section', { className: skip.has('card') ? '' : 'card' }, [form]);
  env.document.body.appendChild(card);
  if (!skip.has('submit-error')) {
    env.document.body.appendChild(h('div', { id: 'submit-error', hidden: true }, add('submit-error-content', h('div', { id: 'submit-error-content' }))));
  }
  return card;
}

// boot builds the page, loads every module plus submit.js, and returns helpers.
function boot(t, opts) {
  const o = opts || {};
  const env = reset(o.url || 'https://gone.test/');
  const card = page(env, o);
  const logs = captureConsole(t);
  FakeXHR.reset();
  globalThis.XMLHttpRequest = FakeXHR;
  load(...(o.modules || MODULES), 'submit');
  const $ = (id) => env.document.getElementById(id);
  const btn = env.document.body.querySelector('button[type="submit"]');
  return { env, card, logs, $, btn, label: btn && btn.querySelector('span') };
}

const submit = (b) => b.$('create-secret').dispatch('submit');
const type = (b, text) => { b.$('secret').value = text; b.$('secret').dispatch('input'); };
const addFiles = (b, files) => { b.$('secret-files').files = files; b.$('secret-files').dispatch('change'); };

test('does nothing when dependencies or required elements are missing', (t) => {
  const a = boot(t, { modules: ['util'] });
  assert.equal(a.btn.disabled, false);
  for (const skip of ['secret', 'ttl', 'button', 'card']) {
    const b = boot(t, { skip: [skip] });
    assert.equal(b.$('size-label').textContent, '', skip);
  }
  const c = reset();
  load(...MODULES, 'submit');
  assert.equal(c.document.body.children.length, 0);
});

test('initial render disables the empty form and shows the meter', (t) => {
  const b = boot(t);
  assert.equal(b.btn.disabled, true);
  assert.equal(b.$('size-label').textContent, '0 B of 1000 B');
  assert.equal(b.$('size-warning').hidden, true);
});

test('typing and attaching files update the meter and button', (t) => {
  const b = boot(t);
  type(b, 'hello');
  assert.equal(b.btn.disabled, false);
  assert.equal(b.$('size-label').textContent, '21 B of 1000 B');
  type(b, '');
  addFiles(b, [new File(['x'.repeat(2000)], 'big.bin')]);
  assert.equal(b.btn.disabled, true);
  assert.match(b.$('size-warning').textContent, /^Over the limit by/);
  b.$('file-list').querySelector('button').click();
  assert.equal(b.btn.disabled, true);
  assert.equal(b.$('size-warning').hidden, true);
  addFiles(b, Array.from({ length: 11 }, (_, i) => new File(['x'], 'f' + i)));
  assert.equal(b.$('size-warning').textContent, 'Too many files: remove 1 to stay within 10.');
});

test('submitting an empty or oversized form shows an error', (t) => {
  const b = boot(t);
  assert.equal(submit(b).defaultPrevented, true);
  assert.equal(b.$('submit-error').hidden, false);
  assert.equal(b.$('submit-error').getAttribute('aria-hidden'), 'false');
  assert.equal(b.$('submit-error-content').textContent, 'Add a message or at least one file');
  type(b, 'x'.repeat(2000));
  submit(b);
  assert.match(b.$('submit-error-content').textContent, /^Over the limit by/);
  assert.equal(FakeXHR.instances.length, 0);
  addFiles(b, [new File(['a'], 'a')]);
  assert.equal(b.$('submit-error').hidden, true);
  assert.equal(b.$('submit-error').getAttribute('aria-hidden'), 'true');
});

test('successful submission encrypts, uploads and shows the share link', async (t) => {
  const b = boot(t);
  const textarea = b.$('secret');
  type(b, 'my secret');
  addFiles(b, [new File(['AB'], 'a.txt', { type: 'text/plain' })]);
  let sent;
  FakeXHR.onSend = (x) => { sent = x; };
  submit(b);
  assert.equal(b.btn.hasAttribute('aria-busy'), true);
  assert.equal(b.$('secret').readOnly, true);
  assert.equal(b.$('secret-files').disabled, true);
  assert.equal(b.btn.disabled, true);
  assert.equal(b.label.textContent, 'Encrypting\u2026');

  submit(b);
  addFiles(b, [new File(['z'], 'late.txt')]);
  b.$('file-list').querySelector('button').click();
  assert.equal(b.$('file-list').children.length, 1);

  await waitFor(() => sent);
  assert.equal(FakeXHR.instances.length, 1);
  assert.equal(sent.headers['X-Gone-TTL'], '3600');
  assert.equal(b.label.textContent, 'Uploading 0%');
  sent.upload.onprogress({ lengthComputable: true, loaded: sent.body.length / 2, total: sent.body.length });
  assert.equal(b.label.textContent, 'Uploading 50%');
  assert.equal(b.$('upload-progress').hidden, false);
  assert.equal(b.$('upload-progress').textContent, '50%');
  const ciphertext = sent.body.slice();
  const nonce = sent.headers['X-Gone-Nonce'];
  sent.respond(201, { id: 'abc123', expires_at: '2030-01-01T00:00:00Z' });

  await waitFor(() => b.env.document.getElementById('share-link'));
  assert.equal(b.card.parentNode, null);
  assert.equal(textarea.value, '');
  const share = new URL(b.$('share-link').value);
  assert.equal(share.pathname, '/secret/abc123');

  const gc = window.goneCrypto;
  const key = gc.importKeyB64(share.hash.replace('#v1:', ''));
  const pt = await gc.decrypt(ciphertext, gc.b64urlDecode(nonce), key);
  const dec = window.goneEnvelope.decode(pt);
  assert.equal(dec.message, 'my secret');
  assert.deepEqual(dec.files.map((f) => f.name), ['a.txt']);
});

test('upload failures restore the form with a friendly error', async (t) => {
  const cases = [
    ['413', (x) => x.respond(413), 'Secret too large for this server'],
    ['network', (x) => x.onerror(), 'Network error uploading secret']
  ];
  for (const [name, finish, msg] of cases) {
    await t.test(name, async (st) => {
      const b = boot(st);
      type(b, 'secret');
      FakeXHR.onSend = finish;
      submit(b);
      await waitFor(() => !b.$('submit-error').hidden);
      assert.equal(b.$('submit-error-content').textContent, msg);
      assert.equal(b.btn.hasAttribute('aria-busy'), false);
      assert.equal(b.$('secret').readOnly, false);
      assert.equal(b.$('secret').value, 'secret');
      assert.equal(b.label.textContent, 'Encrypt');
      assert.equal(b.$('upload-progress').hidden, true);
      assert.equal(b.btn.disabled, false);
      assert.ok(b.logs.error.some((l) => /upload failed/.test(l)));
    });
  }
});

test('an unreadable file reports an encryption failure', async (t) => {
  const b = boot(t);
  const bad = new File(['x'], 'bad.bin');
  bad.arrayBuffer = () => Promise.reject(new Error('NotReadableError'));
  addFiles(b, [bad]);
  submit(b);
  await waitFor(() => !b.$('submit-error').hidden);
  assert.equal(b.$('submit-error-content').textContent, 'Encryption failed: a file could not be read.');
  assert.match(b.logs.error[0], /encryption failed/);
  assert.equal(FakeXHR.instances.length, 0);
});

test('optional elements may be absent', async (t) => {
  const b = boot(t, {
    skip: ['drop-zone', 'secret-files', 'file-list', 'size-meter', 'size-label', 'size-warning', 'upload-progress', 'submit-error'],
    maxBytes: null,
    noLabel: true
  });
  submit(b);
  type(b, 'x'.repeat(5000));
  assert.equal(b.btn.disabled, false);
  FakeXHR.onSend = (x) => { x.upload.onprogress({ lengthComputable: true, loaded: 0, total: 0 }); x.respond(500); };
  submit(b);
  await waitFor(() => b.logs.error.length >= 2);
  assert.equal(b.btn.hasAttribute('aria-busy'), false);
});

test('negative or invalid max bytes means unlimited', (t) => {
  for (const maxBytes of ['-5', 'abc']) {
    const b = boot(t, { maxBytes });
    type(b, 'x'.repeat(5000));
    assert.equal(b.btn.disabled, false, maxBytes);
  }
});

test('secure wipe failure is ignored', async (t) => {
  const b = boot(t);
  type(b, 'abc');
  const ta = b.$('secret');
  let val = ta.value;
  let wipes = 0;
  Object.defineProperty(ta, 'value', {
    get: () => val,
    set: (v) => { if (b.btn.hasAttribute('aria-busy') && wipes++ === 0) throw new Error('readonly'); val = v; }
  });
  FakeXHR.onSend = (x) => x.respond(201, { id: 'i', expires_at: '2030-01-01T00:00:00Z' });
  submit(b);
  await waitFor(() => b.env.document.getElementById('share-link'));
  assert.equal(wipes, 1);
});

test('preview=result renders a mock share panel without focus', (t) => {
  const b = boot(t, { url: 'https://gone.test/?preview=result' });
  const link = b.$('share-link');
  assert.equal(link.value, 'https://gone.test/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa#v1:' + 'A'.repeat(43));
  assert.equal(b.env.document.activeElement, null);
  assert.ok(new Date(b.env.document.body.querySelector('time').getAttribute('datetime')) > new Date());
  const c = boot(t, { url: 'https://gone.test/?preview=other' });
  assert.equal(c.$('share-link'), null);
});
