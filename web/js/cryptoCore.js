'use strict';

// Core v1 protocol primitives shared by v1 and v2 crypto modules.
(function cryptoCoreModule() {
  if (window.goneCryptoCore || !window.goneCryptoEncoding) return;
  const VERSION = 0x01;
  const VERSION_V2 = 0x02;
  const KEY_BYTES = 32;
  const NONCE_BYTES = 12;
  const AAD = 'gone:v1';
  const enc = window.goneCryptoEncoding;

  function randomBytes(n) {
    const b = new Uint8Array(n);
    crypto.getRandomValues(b);
    return b;
  }

  function assertKey(keyBytes) {
    if (!(keyBytes instanceof Uint8Array) || keyBytes.length !== KEY_BYTES) throw new Error('invalid key length');
  }

  function toBytes(data) {
    if (typeof data === 'string') return new TextEncoder().encode(data);
    if (data instanceof Uint8Array) return data;
    throw new Error('data must be string or Uint8Array');
  }

  function gcmParams(nonce, aad) {
    return { name: 'AES-GCM', iv: nonce, additionalData: aad || new TextEncoder().encode(AAD) };
  }

  function importAesKey(raw, usage) {
    return crypto.subtle.importKey('raw', raw, { name: 'AES-GCM' }, false, [usage]);
  }

  function generateKey() {
    return randomBytes(KEY_BYTES);
  }

  function importKeyB64(k) {
    const b = enc.b64urlDecode(k);
    if (b.length !== KEY_BYTES) throw new Error('invalid key');
    return b;
  }

  function codedError(code) {
    const e = new Error(code);
    e.code = code;
    return e;
  }

  window.goneCryptoCore = Object.freeze({ VERSION, VERSION_V2, KEY_BYTES, NONCE_BYTES, randomBytes, assertKey, toBytes, gcmParams, importAesKey, generateKey, importKeyB64, codedError });
})();
