'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { FakeXHR } = require('./fakes');
const { KDF_WAIT, PASS_MODULES, boot, submit, type, typePass, waitFor } = require('./submitHarness');

test('a passphrase seals protocol v2 and marks the result', async (t) => {
  const b = boot(t, { pass: true, modules: PASS_MODULES });
  type(b, 'my secret');
  assert.equal(b.$('size-label').textContent, '25 B of 1000 B');
  typePass(b, 'abc');
  assert.equal(b.$('size-label').textContent, '46 B of 1000 B');
  assert.equal(b.blocked(), true);
  b.$('pass-disclosure').open = false;
  submit(b);
  assert.match(b.$('submit-error-content').textContent, /at least 8 characters, or leave it empty/);
  assert.equal(b.$('pass-disclosure').open, true);
  assert.equal(b.env.document.activeElement, b.$('passphrase'));
  assert.equal(FakeXHR.instances.length, 0);
  typePass(b, 'correct horse battery');
  assert.equal(b.$('submit-error').hidden, true);
  assert.equal(b.blocked(), false);

  let sent;
  FakeXHR.onSend = (x) => { sent = x; };
  submit(b);
  assert.equal(b.$('passphrase').readOnly, true);
  await waitFor(() => sent, KDF_WAIT);
  assert.equal(sent.headers['X-Gone-Version'], '2');
  const blob = sent.body.slice();
  const nonce = sent.headers['X-Gone-Nonce'];
  sent.respond(201, { id: 'i'.repeat(32), expires_at: '2030-01-01T00:00:00Z' });
  await waitFor(() => !b.$('result').hidden);
  assert.equal(b.$('result-pass-note').hidden, false);
  assert.equal(b.$('passphrase').value, '');

  const gc = window.goneCrypto;
  const hash = new URL(b.$('share-link').value).hash;
  assert.match(hash, /^#v2:/);
  const key = gc.importKeyB64(hash.replace('#v2:', ''));
  const pt = await gc.decryptV2(blob, gc.b64urlDecode(nonce), key, 'correct horse battery');
  assert.equal(window.goneEnvelope.decode(pt).message, 'my secret');
});

test('a failed v2 upload keeps the passphrase for retry', async (t) => {
  const b = boot(t, { pass: true, modules: PASS_MODULES });
  type(b, 'secret');
  typePass(b, 'correct horse battery');
  FakeXHR.onSend = (x) => x.respond(500);
  submit(b);
  await waitFor(() => !b.$('submit-error').hidden, KDF_WAIT);
  assert.equal(b.$('passphrase').value, 'correct horse battery');
  assert.equal(b.$('passphrase').readOnly, false);
  assert.equal(b.$('result-pass-note').hidden, true);
});

test('preview=result&passphrase shows a v2 link and the passphrase note', (t) => {
  const b = boot(t, { url: 'https://gone.test/?preview=result&passphrase', pass: true, modules: PASS_MODULES });
  assert.match(b.$('share-link').value, /#v2:A{43}$/);
  assert.equal(b.$('result-pass-note').hidden, false);
  const c = boot(t, { url: 'https://gone.test/?preview=result' });
  assert.equal(c.$('result-pass-note').hidden, true);
});
