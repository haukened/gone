'use strict';

const { reset, load, fastUtil, captureConsole, waitFor, h } = require('./harness');
const { fakeResponse } = require('./fakes');

const ID = '0123456789abcdef0123456789abcdef';
const VALID_FRAG = '#v1:' + 'A'.repeat(43);
const BASE = 'https://gone.test/secret/';
const KDF_WAIT = 15000;
const PASS = 'Correct horse';

function consumeIds() {
  return ['open-heading', 'download-progress', 'consume-status', 'consume-error-text',
    'revealed-heading', 'ack-warning', 'copy-secret', 'secret-output', 'copy-status', 'file-output-list',
    'download-all', 'gone-heading'];
}

function openView(id) {
  return h('div', { id }, [
    h('div', { id: 'open-pass-field', hidden: true }, [h('input', { id: 'open-passphrase', type: 'password' }),
      h('button', { id: 'open-pass-toggle' }, [h('span', { textContent: 'Show' })])]),
    h('p', { id: 'open-pass-warn', hidden: true }),
    h('button', { id: 'open-secret' }, [h('span', { textContent: 'Open secret' })]),
    h('div', { id: 'consume-error', hidden: true })
  ]);
}

function buildPage(env, opts) {
  const open = openView(opts.noPage ? 'other' : 'view-open');
  const revealed = h('div', { id: 'view-revealed', hidden: true }, [h('div', { id: 'message-panel', hidden: true }),
    h('div', { id: 'file-section', hidden: true })]);
  const gone = h('div', { id: 'view-gone', hidden: true });
  consumeIds().forEach((id) => open.appendChild(h(id === 'file-output-list' ? 'ul' : 'div', { id, hidden: id === 'ack-warning' || id === 'download-progress' })));
  env.document.body.append(open, revealed, gone);
}

function boot(t, url, opts) {
  const o = opts || {};
  const env = reset(url);
  buildPage(env, o);
  const logs = captureConsole(t);
  load('util', 'crypto', 'fileMeta', 'envelope', 'icons');
  fastUtil();
  load(...(o.modules || ['consumeApi', 'consumeView', 'consume']));
  const $ = (id) => env.document.getElementById(id);
  return { env, logs, $, open: () => $('open-secret').click() };
}

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

function typePass($, value) {
  $('open-passphrase').value = value;
  $('open-passphrase').dispatch('input');
}

const disabled = ($) => $('open-secret').getAttribute('aria-disabled') === 'true';

module.exports = { ID, VALID_FRAG, BASE, KDF_WAIT, PASS, boot, sealed, sealedV2, typePass, disabled, reset, load, waitFor };
