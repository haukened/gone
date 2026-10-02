'use strict';

// AES-256-GCM v1 primitives: 32-byte keys, fresh 12-byte nonces, and the
// static AAD "gone:v1". Exposed as window.goneCrypto.
(function cryptoScaffold() {
  const VERSION = 0x01;
  const KEY_BYTES = 32;
  const NONCE_BYTES = 12;
  const AAD = 'gone:v1';
  if (window.goneCrypto) return;

  function b64urlEncode(bytes) {
    let bin = '';
    for (let i = 0; i < bytes.length; i++) {
      bin += String.fromCharCode(bytes[i]);
    }
    const b64 = btoa(bin).replace(/\+/g, '-').replace(/\//g, '_');
    let end = b64.length;
    while (end > 0 && b64.charAt(end - 1) === '=') {
      end--;
    }
    return b64.substring(0, end);
  }

  function b64urlDecode(s) {
    let norm = s.replace(/-/g, '+').replace(/_/g, '/');
    while (norm.length % 4) {
      norm += '=';
    }
    const bin = atob(norm);
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) {
      out[i] = bin.charCodeAt(i);
    }
    return out;
  }

  function randomBytes(n) {
    const b = new Uint8Array(n);
    crypto.getRandomValues(b);
    return b;
  }

  function assertKey(keyBytes) {
    if (!(keyBytes instanceof Uint8Array) || keyBytes.length !== KEY_BYTES) {
      throw new Error('invalid key length');
    }
  }

  // toBytes accepts a string (UTF-8 encoded) or a Uint8Array as plaintext.
  function toBytes(data) {
    if (typeof data === 'string') return new TextEncoder().encode(data);
    if (data instanceof Uint8Array) return data;
    throw new Error('data must be string or Uint8Array');
  }

  function gcmParams(nonce) {
    return { name: 'AES-GCM', iv: nonce, additionalData: new TextEncoder().encode(AAD) };
  }

  function importKey(raw, usage) {
    return crypto.subtle.importKey('raw', raw, { name: 'AES-GCM' }, false, [usage]);
  }

  function generateKey() {
    return randomBytes(KEY_BYTES);
  }

  // encrypt seals data under keyBytes with a fresh random nonce.
  async function encrypt(data, keyBytes) {
    assertKey(keyBytes);
    const plaintextBytes = toBytes(data);
    const key = await importKey(keyBytes, 'encrypt');
    const nonce = randomBytes(NONCE_BYTES);
    const ctBuf = await crypto.subtle.encrypt(gcmParams(nonce), key, plaintextBytes);
    return { nonce: nonce, ciphertext: new Uint8Array(ctBuf) };
  }

  // decrypt opens ciphertext sealed by encrypt; it rejects if the key,
  // nonce, AAD, or ciphertext do not authenticate.
  async function decrypt(ciphertext, nonce, keyBytes) {
    assertKey(keyBytes);
    if (nonce.length !== NONCE_BYTES) throw new Error('invalid nonce length');
    const key = await importKey(keyBytes, 'decrypt');
    const pt = await crypto.subtle.decrypt(gcmParams(nonce), key, ciphertext);
    return new Uint8Array(pt);
  }

  function exportKeyB64(k) {
    return b64urlEncode(k);
  }

  function importKeyB64(k) {
    const b = b64urlDecode(k);
    if (b.length !== KEY_BYTES) throw new Error('invalid key');
    return b;
  }

  window.goneCrypto = Object.freeze({
    version: VERSION,
    generateKey: generateKey,
    exportKeyB64: exportKeyB64,
    importKeyB64: importKeyB64,
    encrypt: encrypt,
    decrypt: decrypt,
    b64urlEncode: b64urlEncode,
    b64urlDecode: b64urlDecode
  });
  console.log('Gone crypto module loaded');
})();
