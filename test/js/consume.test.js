'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, fastUtil, captureConsole, waitFor, h } = require('./harness');
const { fakeResponse, installFetch } = require('./fakes');

const ID = '0123456789abcdef0123456789abcdef';
const VALID_FRAG = '#v1:' + 'A'.repeat(43);
const BASE = 'https://gone.test/secret/';
// KDF_WAIT bounds waits that include a 600k-iteration PBKDF2 under a loaded runner.
const KDF_WAIT = 15000;
const PASS = 'Correct horse';

// boot builds the consume page at url, loads every module and runs consume.js.
function boot(t, url, opts) {
  const o = opts || {};
  const env = reset(url);
  const ids = ['open-heading', 'download-progress', 'consume-status', 'consume-error-text',
    'revealed-heading', 'ack-warning', 'copy-secret', 'secret-output', 'copy-status', 'file-output-list',
    'download-all', 'gone-heading'];
  const open = h('div', { id: o.noPage ? 'other' : 'view-open' }, [
    h('div', { id: 'open-pass-field', hidden: true }, [h('input', { id: 'open-passphrase', type: 'password' }),
      h('button', { id: 'open-pass-toggle' }, [h('span', { textContent: 'Show' })])]),
    h('p', { id: 'open-pass-warn', hidden: true }),
    h('button', { id: 'open-secret' }, [h('span', { textContent: 'Open secret' })]),
    h('div', { id: 'consume-error', hidden: true })
  ]);
  const revealed = h('div', { id: 'view-revealed', hidden: true }, [h('div', { id: 'message-panel', hidden: true }),
    h('div', { id: 'file-section', hidden: true })]);
  const gone = h('div', { id: 'view-gone', hidden: true });
  ids.forEach((id) => open.appendChild(h(id === 'file-output-list' ? 'ul' : 'div', { id, hidden: id === 'ack-warning' || id === 'download-progress' })));
  env.document.body.append(open, revealed, gone);
  const logs = captureConsole(t);
  load('util', 'crypto', 'fileMeta', 'envelope', 'icons');
  fastUtil();
  load(...(o.modules || ['consumeApi', 'consumeView', 'consume']));
  const $ = (id) => env.document.getElementById(id);
  return { env, logs, $, open: () => $('open-secret').click() };
}

// sealed encrypts plaintext under a new key and returns fetch handlers.
async function sealed(plaintext) {
  reset();
  load('crypto');
  const gc = window.goneCrypto;
  const key = gc.generateKey();
  const enc = await gc.encrypt(plaintext, key);
  const headers = (len) => ({
    'Content-Length': String(len), 'X-Gone-Claim': 'claim-1',
    'X-Gone-Version': '1', 'X-Gone-Nonce': gc.b64urlEncode(enc.nonce)
  });
  return {
    frag: `#v1:${gc.exportKeyB64(key)}`,
    get: () => fakeResponse({ headers: headers(enc.ciphertext.length), chunks: [enc.ciphertext.slice()] }),
    short: () => fakeResponse({ headers: headers(enc.ciphertext.length + 1), chunks: [enc.ciphertext.slice()] })
  };
}

// sealedV2 encrypts plaintext under a new link key and PASS and returns
// fetch handlers; corrupt damages the KDF header.
async function sealedV2(plaintext, corrupt) {
  reset();
  load('crypto');
  const gc = window.goneCrypto;
  const key = gc.generateKey();
  const enc = await gc.encryptV2(plaintext, key, PASS);
  if (corrupt) enc.ciphertext[0] = 9;
  const headers = {
    'Content-Length': String(enc.ciphertext.length), 'X-Gone-Claim': 'claim-1',
    'X-Gone-Version': '2', 'X-Gone-Nonce': gc.b64urlEncode(enc.nonce)
  };
  return {
    frag: `#v2:${gc.exportKeyB64(key)}`,
    get: () => fakeResponse({ headers, chunks: [enc.ciphertext.slice()] })
  };
}

// typePass enters a passphrase the way a user would.
function typePass($, value) {
  $('open-passphrase').value = value;
  $('open-passphrase').dispatch('input');
}

const disabled = ($) => $('open-secret').getAttribute('aria-disabled') === 'true';

test('does nothing without dependencies or the consume page', (t) => {
  reset();
  load('consume');
  const calls = installFetch([]);
  const a = boot(t, BASE + ID + VALID_FRAG, { noPage: true });
  a.open();
  assert.equal(a.env.windowListeners.beforeunload, undefined);
  const b = boot(t, BASE + ID + VALID_FRAG, { modules: ['consumeView', 'consume'] });
  b.open();
  assert.equal(b.env.windowListeners.beforeunload, undefined);
  assert.equal(calls.length, 0);
});

test('preview mode shows sample or custom text without fetching', (t) => {
  const calls = installFetch([]);
  const a = boot(t, BASE + 'x?preview=secret');
  assert.match(a.$('secret-output').textContent, /preview of a decrypted secret/);
  assert.equal(a.$('view-revealed').hidden, false);
  const b = boot(t, BASE + 'x?preview=secret&text=hi%20there');
  assert.equal(b.$('secret-output').textContent, 'hi there');
  assert.equal(calls.length, 0);
});

test('fragment and id problems are reported and lock Open without fetching', (t) => {
  const calls = installFetch([]);
  const BAD = /everything after the #/;
  const cases = [
    [BASE + ID, BAD],
    [BASE + ID + '#v1:short', BAD],
    [BASE + ID + '#v1:bad+chars/xx', BAD],
    [BASE + ID + '#v1:' + 'A'.repeat(42) + 'B', BAD],
    [BASE + ID + '#v01:' + 'A'.repeat(43), BAD],
    [BASE + ID + '#v3:' + 'A'.repeat(43), /newer version of Gone/],
    [BASE + 'not-an-id' + VALID_FRAG, /isn\u2019t valid/]
  ];
  for (const [url, want] of cases) {
    const { $, open } = boot(t, url);
    assert.match($('consume-error-text').textContent, want, url);
    assert.equal($('consume-error').hidden, false);
    assert.ok(disabled($), url);
    open();
  }
  assert.equal(calls.length, 0);
});

test('nothing is fetched until Open; then fetch, decrypt, decode files and acknowledge', async (t) => {
  const plain = await (async () => {
    reset();
    load('fileMeta', 'envelope');
    return window.goneEnvelope.encode('top secret', [{ name: 'a.txt', type: 'text/plain', bytes: new Uint8Array([65]) }]);
  })();
  const s = await sealed(plain);
  const calls = installFetch([s.get, () => fakeResponse({ status: 204 })]);
  const { $, env, open } = boot(t, BASE + ID + s.frag);
  assert.equal(calls.length, 0);
  assert.equal(env.windowListeners.beforeunload, undefined);
  open();
  open();
  assert.equal($('open-secret').getAttribute('aria-busy'), 'true');
  assert.equal(typeof env.windowListeners.beforeunload, 'function');
  assert.equal(typeof env.windowListeners.pagehide, 'function');
  await waitFor(() => calls.length === 2);
  await waitFor(() => $('view-revealed').hidden === false);
  assert.equal($('revealed-heading').textContent, 'Here\u2019s your secret and a file.');
  assert.equal($('secret-output').textContent, 'top secret');
  assert.equal($('file-output-list').children.length, 1);
  assert.equal($('ack-warning').hidden, true);
  assert.equal(calls[0].url, `https://gone.test/api/secret/${ID}`);
  assert.equal(calls[1].init.method, 'DELETE');
  assert.deepEqual(calls[1].init.headers, { 'X-Gone-Claim': 'claim-1' });
  open();
  assert.equal(calls.length, 2);
  const ev = { preventDefault() { this.prevented = true; } };
  env.windowListeners.beforeunload(ev);
  assert.equal(ev.prevented, true);
  env.windowListeners.pagehide();
});

test('a failed acknowledgement shows the warning', async (t) => {
  const s = await sealed('just text');
  installFetch([s.get, ...Array(3).fill(() => fakeResponse({ status: 500 }))]);
  const { $, open } = boot(t, BASE + ID + s.frag);
  open();
  await waitFor(() => !$('ack-warning').hidden);
  assert.equal($('revealed-heading').textContent, 'Here\u2019s your secret.');
  assert.equal($('secret-output').textContent, 'just text');
});

test('a retryable failure leaves Open usable and the retry reuses the claim', async (t) => {
  const s = await sealed('again');
  const calls = installFetch([s.short, s.short, s.short, s.get, () => fakeResponse({ status: 204 })]);
  const { $, logs, open } = boot(t, BASE + ID + s.frag);
  open();
  await waitFor(() => !$('consume-error').hidden);
  assert.match($('consume-error-text').textContent, /interrupted/);
  assert.equal(disabled($), false);
  assert.equal($('open-secret').hasAttribute('aria-busy'), false);
  assert.ok(logs.error.length > 0);
  open();
  assert.equal($('consume-error').hidden, true);
  await waitFor(() => calls.length === 5);
  assert.deepEqual(calls[3].init.headers, { 'X-Gone-Claim': 'claim-1' });
  await waitFor(() => $('view-revealed').hidden === false);
});

test('final failures lock Open; gone shows the gone view', async (t) => {
  const s = await sealed('x');
  const locked = (re) => (d) => {
    assert.match(d.$('consume-error-text').textContent, re);
    assert.ok(disabled(d.$));
  };
  const gone = (d) => {
    assert.equal(d.$('view-gone').hidden, false);
    assert.equal(d.$('consume-error').hidden, true);
    assert.equal(d.logs.error.length, 0);
  };
  const cases = [
    ['wrong key', [s.get], '#v1:' + 'A'.repeat(43), locked(/Couldn.t verify/)],
    ['bad request', [() => fakeResponse({ status: 400 })], s.frag, locked(/isn\u2019t valid/)],
    ['unexpected', [() => ({ ok: true, status: 200, headers: { get() { throw new Error('boom'); } } })], s.frag,
      locked(/^Something went wrong opening this secret\. Try again\.$/)],
    ['gone 404', [() => fakeResponse({ status: 404 })], s.frag, gone],
    ['gone 410', [() => fakeResponse({ status: 410 })], s.frag, gone]
  ];
  for (const [name, handlers, frag, check] of cases) {
    await t.test(name, async (st) => {
      const calls = installFetch(handlers);
      const d = boot(st, BASE + ID + frag);
      d.open();
      await waitFor(() => d.logs.error.length > 0 || d.$('view-gone').hidden === false);
      check(d);
      assert.equal(d.$('download-progress').hidden, true);
      d.open();
      assert.equal(calls.length, 1);
    });
  }
});

test('a malformed envelope is reported and not acknowledged', async (t) => {
  const bad = new Uint8Array([0x47, 0x4f, 0x4e, 0x45, 0x32, 0, 0, 0, 0, 0, 0, 2, 0x7b, 0x7d]);
  const s = await sealed(bad);
  const calls = installFetch([s.get]);
  const { $, logs, open } = boot(t, BASE + ID + s.frag);
  open();
  await waitFor(() => logs.error.length >= 2);
  assert.match(logs.error[0], /invalid envelope/);
  assert.equal($('consume-error-text').textContent, 'This secret\u2019s contents are damaged. Ask the sender to share it again.');
  assert.ok(disabled($));
  assert.equal(calls.length, 1);
});

test('a v2 link asks for the passphrase and retries a wrong one without fetching again', async (t) => {
  const s = await sealedV2('v2 secret');
  const calls = installFetch([s.get, () => fakeResponse({ status: 204 })]);
  const { $, env, logs, open } = boot(t, BASE + ID + s.frag);
  assert.equal($('open-pass-field').hidden, false);
  assert.equal($('open-pass-warn').hidden, false);
  assert.ok(disabled($));
  open();
  assert.equal(env.document.activeElement, $('open-passphrase'));
  assert.equal(calls.length, 0);

  typePass($, 'wrong horse');
  assert.equal(disabled($), false);
  open();
  await waitFor(() => logs.error.length > 0, KDF_WAIT);
  assert.equal($('consume-error-text').textContent, 'That passphrase didn\u2019t work. Check it and try again.');
  assert.equal($('open-passphrase').getAttribute('aria-invalid'), 'true');
  assert.equal($('open-secret').textContent, 'Try again');
  assert.equal(disabled($), false);
  assert.equal(calls.length, 1);

  typePass($, PASS);
  assert.equal($('open-passphrase').getAttribute('aria-invalid'), 'false');
  open();
  assert.equal($('consume-status').textContent, 'Unlocking\u2026');
  await waitFor(() => calls.length === 2, KDF_WAIT);
  assert.equal(calls[1].init.method, 'DELETE');
  assert.equal($('secret-output').textContent, 'v2 secret');
  assert.equal($('open-passphrase').value, '');
  env.windowListeners.pagehide();
  assert.equal($('view-revealed').hidden, false);
});

test('a damaged v2 header is final and erases the download', async (t) => {
  const s = await sealedV2('x', true);
  const calls = installFetch([s.get]);
  const { $, logs, open } = boot(t, BASE + ID + s.frag);
  typePass($, PASS);
  open();
  await waitFor(() => logs.error.length > 0, KDF_WAIT);
  assert.match($('consume-error-text').textContent, /contents are damaged/);
  assert.ok(disabled($));
  assert.equal($('open-passphrase').readOnly, true);
  open();
  assert.equal(calls.length, 1);
});

test('leaving the page erases a download held for a passphrase retry', async (t) => {
  const s = await sealedV2('x');
  const calls = installFetch([s.get]);
  const { $, env, logs, open } = boot(t, BASE + ID + s.frag);
  typePass($, 'wrong horse');
  open();
  await waitFor(() => logs.error.length > 0, KDF_WAIT);
  env.windowListeners.pagehide();
  assert.match($('consume-error-text').textContent, /You left this page/);
  assert.ok(disabled($));
  assert.equal($('open-passphrase').value, '');
  typePass($, PASS);
  open();
  assert.equal(calls.length, 1);
  env.windowListeners.pagehide();
});

test('leaving the page mid-retry erases the download and ignores the outcome', async (t) => {
  const s = await sealedV2('x');
  const calls = installFetch([s.get]);
  const { $, env, logs, open } = boot(t, BASE + ID + s.frag);
  typePass($, 'wrong horse');
  open();
  await waitFor(() => logs.error.length > 0, KDF_WAIT);
  typePass($, PASS);
  open();
  env.windowListeners.pagehide();
  await waitFor(() => !$('open-secret').hasAttribute('aria-busy'), KDF_WAIT);
  assert.match($('consume-error-text').textContent, /You left this page/);
  assert.equal(logs.error.length, 1);
  assert.ok(disabled($));
  assert.equal($('view-revealed').hidden, true);
  assert.equal(calls.length, 1);
});
