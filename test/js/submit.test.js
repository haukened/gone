'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { FakeXHR } = require('./fakes');
const { MODULES, boot, submit, type, addFiles, reset, load, waitFor } = require('./submitHarness');

test('does nothing when dependencies or required elements are missing', (t) => {
  const a = boot(t, { modules: ['util'] });
  assert.equal(a.btn.getAttribute('aria-disabled'), null);
  for (const skip of ['secret', 'button', 'create-secret']) {
    const b = boot(t, { skip: [skip] });
    assert.equal(b.$('size-label') ? b.$('size-label').textContent : '', '', skip);
  }
  const c = reset();
  load(...MODULES, 'submit');
  assert.equal(c.document.body.children.length, 0);
});

test('initial render blocks the empty form and shows the meter', (t) => {
  const b = boot(t);
  assert.equal(b.blocked(), true);
  assert.equal(b.$('size-label').textContent, '0 B of 1000 B');
  assert.equal(b.$('size-warning').hidden, true);
  assert.equal(b.$('size-meter').max, 1000);
});

test('typing and attaching files update the meter and button', (t) => {
  const b = boot(t);
  type(b, 'hello');
  assert.equal(b.blocked(), false);
  assert.equal(b.$('size-label').textContent, '21 B of 1000 B');
  type(b, '');
  addFiles(b, [new File(['x'.repeat(2000)], 'big.bin')]);
  assert.equal(b.blocked(), true);
  assert.ok(b.$('size-box').classList.contains('over'));
  assert.match(b.$('size-warning-text').textContent, /^Over the limit by/);
  b.$('file-list').querySelector('button').click();
  assert.equal(b.blocked(), true);
  assert.equal(b.$('size-warning').hidden, true);
  assert.equal(b.env.document.activeElement, b.$('secret-files'));
  addFiles(b, Array.from({ length: 11 }, (_, i) => new File(['x'], 'f' + i)));
  assert.equal(b.$('size-warning-text').textContent, 'Too many files: remove 1 to stay within 10.');
});

test('submitting an empty or oversized form explains why', (t) => {
  const b = boot(t);
  assert.equal(submit(b).defaultPrevented, true);
  assert.equal(b.$('submit-error').hidden, false);
  assert.equal(b.$('submit-error-content').textContent, 'Add a message or at least one file.');
  type(b, 'x'.repeat(2000));
  submit(b);
  assert.match(b.$('submit-error-content').textContent, /^Over the limit by/);
  assert.equal(FakeXHR.instances.length, 0);
  addFiles(b, [new File(['a'], 'a')]);
  assert.equal(b.$('submit-error').hidden, true);
});

test('successful submission encrypts, uploads and shows the share link', async (t) => {
  const b = boot(t);
  const textarea = b.$('secret');
  type(b, 'my secret');
  addFiles(b, [new File(['AB'], 'a.txt', { type: 'text/plain' })]);
  let sent;
  FakeXHR.onSend = (x) => { sent = x; };
  submit(b);
  assert.equal(b.btn.getAttribute('aria-busy'), 'true');
  assert.equal(b.$('secret').readOnly, true);
  assert.equal(b.$('secret-files').disabled, true);
  assert.equal(b.blocked(), true);
  assert.equal(b.label.textContent, 'Encrypting\u2026');

  submit(b);
  addFiles(b, [new File(['z'], 'late.txt')]);
  b.$('file-list').querySelector('button').click();
  assert.equal(b.$('file-list').children.length, 1);

  await waitFor(() => sent);
  assert.equal(FakeXHR.instances.length, 1);
  assert.equal(sent.headers['X-Gone-TTL'], '1h');
  assert.equal(b.label.textContent, 'Uploading 0%');
  sent.upload.onprogress({ lengthComputable: true, loaded: 0, total: 0 });
  assert.equal(b.$('upload-progress').max, 1);
  sent.upload.onprogress({ lengthComputable: true, loaded: sent.body.length / 2, total: sent.body.length });
  assert.equal(b.label.textContent, 'Uploading 50%');
  assert.equal(b.$('upload-progress').hidden, false);
  assert.equal(b.$('upload-progress').textContent, '50%');
  const ciphertext = sent.body.slice();
  const nonce = sent.headers['X-Gone-Nonce'];
  const ID = '0123456789abcdef0123456789abcdef';
  sent.respond(201, { id: ID, expires_at: '2030-01-01T00:00:00Z', manage_token: 'M'.repeat(43) });

  await waitFor(() => !b.$('result').hidden);
  assert.equal(b.$('manage-disclosure').hidden, false);
  assert.equal(b.$('manage-link').value, `https://gone.test/manage/${ID}#${'M'.repeat(43)}`);
  assert.equal(b.$('compose').hidden, true);
  assert.equal(b.env.document.activeElement, b.$('result-heading'));
  assert.equal(textarea.value, '');
  const share = new URL(b.$('share-link').value);
  assert.equal(share.pathname, '/secret/' + ID);

  const gc = window.goneCrypto;
  const key = gc.importKeyB64(share.hash.replace('#v1:', ''));
  const pt = await gc.decrypt(ciphertext, gc.b64urlDecode(nonce), key);
  const dec = window.goneEnvelope.decode(pt);
  assert.equal(dec.message, 'my secret');
  assert.deepEqual(dec.files.map((f) => f.name), ['a.txt']);
});

test('upload failures restore the form with a friendly error', async (t) => {
  const cases = [
    ['413', (x) => x.respond(413), /too large/i],
    ['network', (x) => x.onerror(), /reach|network|connection/i]
  ];
  for (const [name, finish, msg] of cases) {
    await t.test(name, async (st) => {
      const b = boot(st);
      type(b, 'secret');
      FakeXHR.onSend = finish;
      submit(b);
      await waitFor(() => !b.$('submit-error').hidden);
      assert.match(b.$('submit-error-content').textContent, msg);
      assert.equal(b.btn.hasAttribute('aria-busy'), false);
      assert.equal(b.$('secret').readOnly, false);
      assert.equal(b.$('secret').value, 'secret');
      assert.equal(b.label.textContent, 'Create one-time link');
      assert.equal(b.$('upload-progress').hidden, true);
      assert.equal(b.blocked(), false);
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
    skip: ['drop-zone', 'secret-files', 'file-list', 'size-box', 'size-meter', 'size-label', 'size-warning', 'upload-progress', 'submit-error'],
    maxBytes: null,
    noLabel: true,
    ttl: null
  });
  submit(b);
  type(b, 'x'.repeat(5000));
  assert.equal(b.blocked(), false);
  let sent;
  FakeXHR.onSend = (x) => { sent = x; x.upload.onprogress({ lengthComputable: true, loaded: 0, total: 0 }); x.respond(500); };
  submit(b);
  await waitFor(() => b.logs.error.length >= 2);
  assert.equal(sent.headers['X-Gone-TTL'], '');
  assert.equal(b.btn.hasAttribute('aria-busy'), false);
});

test('negative or invalid max bytes means unlimited', (t) => {
  for (const maxBytes of ['-5', 'abc']) {
    const b = boot(t, { maxBytes });
    type(b, 'x'.repeat(5000));
    assert.equal(b.blocked(), false, maxBytes);
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
  await waitFor(() => !b.$('result').hidden);
  assert.equal(wipes, 1);
  assert.equal(b.$('manage-disclosure').hidden, true, 'no manage_token hides the manage section');
});

test('preview=result renders a mock share panel without focus', (t) => {
  const b = boot(t, { url: 'https://gone.test/?preview=result' });
  assert.equal(b.$('share-link').value, 'https://gone.test/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa#v1:' + 'A'.repeat(43));
  assert.equal(b.$('result').hidden, false);
  assert.equal(b.env.document.activeElement, null);
  assert.ok(new Date(b.$('result-expiry').getAttribute('datetime')) > new Date());
  assert.equal(b.$('manage-link').value, 'https://gone.test/manage/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa#' + 'B'.repeat(43));
  assert.equal(b.$('manage-disclosure').hidden, false);
  const c = boot(t, { url: 'https://gone.test/?preview=other' });
  assert.equal(c.$('result').hidden, true);
});


test('an insecure page without WebCrypto explains HTTPS instead of encrypting', async (t) => {
  const b = boot(t);
  const real = Object.getOwnPropertyDescriptor(globalThis, 'crypto');
  Object.defineProperty(globalThis, 'crypto', { configurable: true, value: {} });
  t.after(() => Object.defineProperty(globalThis, 'crypto', real));
  b.$('secret').value = 'hello';
  submit(b);
  await waitFor(() => !b.$('submit-error').hidden);
  assert.match(b.$('submit-error-content').textContent, /only encrypts on secure \(HTTPS\) pages/);
  assert.match(b.logs.error[0], /WebCrypto unavailable/);
  assert.equal(FakeXHR.instances.length, 0);
});
