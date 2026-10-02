'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, fastUtil, captureConsole, waitFor, h } = require('./harness');
const { fakeResponse, installFetch } = require('./fakes');

const ID = '0123456789abcdef0123456789abcdef';

// boot builds the consume page at url, loads every module and runs consume.js.
function boot(t, url, opts) {
  const o = opts || {};
  const env = reset(url);
  const ids = ['secret-heading', 'secret-output', 'copy-secret', 'download-all', 'download-progress', 'file-section',
    'file-output-list', 'ack-banner', 'ack-card', 'ack-title', 'ack-text'];
  const root = h('section', { id: o.noPage ? 'other' : 'secret-consume' });
  ids.forEach((id) => root.appendChild(h(id === 'secret-output' ? 'textarea' : 'div', { id, hidden: true })));
  env.document.body.appendChild(root);
  const logs = captureConsole(t);
  load('util', 'crypto', 'fileMeta', 'envelope', 'icons');
  fastUtil();
  load(...(o.modules || ['consumeApi', 'consumeView', 'consume']));
  return { env, logs, $: (id) => env.document.getElementById(id) };
}

// sealed encrypts plaintext under a new key and returns fetch handlers.
async function sealed(plaintext) {
  reset();
  load('crypto');
  const gc = window.goneCrypto;
  const key = gc.generateKey();
  const enc = await gc.encrypt(plaintext, key);
  return {
    keyB64: gc.exportKeyB64(key),
    get: () => fakeResponse({
      headers: {
        'Content-Length': String(enc.ciphertext.length), 'X-Gone-Claim': 'claim-1',
        'X-Gone-Version': '1', 'X-Gone-Nonce': gc.b64urlEncode(enc.nonce)
      },
      chunks: [enc.ciphertext.slice()]
    })
  };
}

test('does nothing without dependencies or the consume page', (t) => {
  reset();
  load('consume');
  const { $, env } = boot(t, 'https://gone.test/secret/' + ID + '#v1:abcdefghijkl', { noPage: true });
  assert.equal($('secret-heading').textContent, '');
  assert.equal(env.windowListeners.beforeunload, undefined);
  const b = boot(t, 'https://gone.test/secret/' + ID + '#v1:abcdefghijkl', { modules: ['consumeView', 'consume'] });
  assert.equal(b.$('secret-heading').textContent, '');
});

test('preview mode shows sample or custom text without fetching', (t) => {
  const calls = installFetch([]);
  const a = boot(t, 'https://gone.test/secret/x?preview=secret');
  assert.match(a.$('secret-output').value, /preview of a decrypted secret/);
  assert.equal(a.$('secret-heading').textContent, 'Decrypted (preview)');
  const b = boot(t, 'https://gone.test/secret/x?preview=secret&text=hi%20there');
  assert.equal(b.$('secret-output').value, 'hi there');
  assert.equal(calls.length, 0);
});

test('fragment and id problems are reported without fetching', (t) => {
  const calls = installFetch([]);
  const cases = [
    ['https://gone.test/secret/' + ID, 'Missing or invalid key fragment. Cannot decrypt.'],
    ['https://gone.test/secret/' + ID + '#v1:short', 'Missing or invalid key fragment. Cannot decrypt.'],
    ['https://gone.test/secret/' + ID + '#v1:bad+chars/xx', 'Missing or invalid key fragment. Cannot decrypt.'],
    ['https://gone.test/secret/' + ID + '#v2:abcdefghijkl', 'Unsupported version'],
    ['https://gone.test/secret/not-an-id#v1:abcdefghijkl', 'Invalid secret id']
  ];
  for (const [url, want] of cases) {
    const { $ } = boot(t, url);
    assert.equal($('secret-heading').textContent, want, url);
  }
  assert.equal(calls.length, 0);
});

test('happy path: fetch, decrypt, decode files, then acknowledge', async (t) => {
  const plain = await (async () => {
    reset();
    load('fileMeta', 'envelope');
    return window.goneEnvelope.encode('top secret', [{ name: 'a.txt', type: 'text/plain', bytes: new Uint8Array([65]) }]);
  })();
  const s = await sealed(plain);
  const calls = installFetch([s.get, () => fakeResponse({ status: 204 })]);
  const { $, env } = boot(t, `https://gone.test/secret/${ID}#v1:${s.keyB64}`);
  assert.equal(typeof env.windowListeners.beforeunload, 'function');
  assert.equal(typeof env.windowListeners.pagehide, 'function');
  await waitFor(() => !$('ack-banner').hidden);
  assert.equal($('secret-heading').textContent, 'Decrypted Secret + 1 file:');
  assert.equal($('secret-output').value, 'top secret');
  assert.equal($('file-output-list').children.length, 1);
  assert.equal($('ack-card').classList.contains('danger'), false);
  assert.equal(calls[0].url, `https://gone.test/api/secret/${ID}`);
  assert.equal(calls[1].init.method, 'DELETE');
  assert.deepEqual(calls[1].init.headers, { 'X-Gone-Claim': 'claim-1' });
  const ev = { preventDefault() { this.prevented = true; } };
  env.windowListeners.beforeunload(ev);
  assert.equal(ev.prevented, true);
  env.windowListeners.pagehide();
});

test('text-only secret with failed acknowledgement shows a warning', async (t) => {
  const s = await sealed('just text');
  installFetch([s.get, ...Array(3).fill(() => fakeResponse({ status: 500 }))]);
  const { $ } = boot(t, `https://gone.test/secret/${ID}#v1:${s.keyB64}`);
  await waitFor(() => !$('ack-banner').hidden);
  assert.equal($('secret-heading').textContent, 'Decrypted Secret:');
  assert.equal($('secret-output').value, 'just text');
  assert.ok($('ack-card').classList.contains('danger'));
});

test('known errors show their message; unexpected ones are generic', async (t) => {
  const s = await sealed('x');
  const cases = [
    ['gone', [() => fakeResponse({ status: 404 })], `#v1:${s.keyB64}`, /This secret is gone/],
    ['bad key length', [s.get], '#v1:AAAAAAAAAAAA', /^Unexpected error$/],
    ['wrong key', [s.get], '#v1:' + 'A'.repeat(43), /Couldn.t verify/]
  ];
  for (const [name, handlers, frag, want] of cases) {
    await t.test(name, async (st) => {
      const calls = installFetch(handlers);
      const { $, logs } = boot(st, `https://gone.test/secret/${ID}${frag}`);
      await waitFor(() => logs.error.length > 0);
      assert.match($('secret-heading').textContent, want);
      assert.equal($('download-progress').hidden, true);
      assert.equal(calls.length, 1);
    });
  }
});

test('a malformed envelope is reported and not acknowledged', async (t) => {
  const bad = new Uint8Array([0x47, 0x4f, 0x4e, 0x45, 0x32, 0, 0, 0, 0, 0, 0, 2, 0x7b, 0x7d]);
  const s = await sealed(bad);
  const calls = installFetch([s.get]);
  const { $, logs } = boot(t, `https://gone.test/secret/${ID}#v1:${s.keyB64}`);
  await waitFor(() => logs.error.length >= 2);
  assert.match(logs.error[0], /invalid envelope/);
  assert.equal($('secret-heading').textContent, 'This secret\u2019s contents are malformed; ask the sender to resend it.');
  assert.equal(calls.length, 1);
});
