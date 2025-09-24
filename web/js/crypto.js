'use strict';

// AES-GCM v1 crypto utilities extracted from app.js
(function cryptoScaffold() {
  const VERSION = 0x01;
  const KEY_BYTES = 32;
  const NONCE_BYTES = 12;
  if (window.goneCrypto) return;
  function utf8Encode(str) { return new TextEncoder().encode(str); }
  function b64urlEncode(bytes) {
    let bin = ''; for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
    const b64 = btoa(bin).replace(/\+/g, '-').replace(/\//g, '_');
    let end = b64.length; while (end > 0 && b64.charAt(end - 1) === '=') end--; return b64.substring(0, end);
  }
  function b64urlDecode(s) {
    let norm = s.replace(/-/g, '+').replace(/_/g, '/'); while (norm.length % 4) norm += '=';
    const bin = atob(norm); const out = new Uint8Array(bin.length); for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i); return out;
  }
  function randomBytes(n) { const b = new Uint8Array(n); crypto.getRandomValues(b); return b; }
  async function importKey(raw) { return crypto.subtle.importKey('raw', raw, { name: 'AES-GCM' }, false, ['encrypt', 'decrypt']); }
  function generateKey() { return randomBytes(KEY_BYTES); }
  async function encrypt(data, keyBytes) {
    if (!(keyBytes instanceof Uint8Array) || keyBytes.length !== KEY_BYTES) throw new Error('invalid key length');
    let plaintextBytes; if (typeof data === 'string') plaintextBytes = utf8Encode(data); else if (data instanceof Uint8Array) plaintextBytes = data; else throw new Error('data must be string or Uint8Array');
    const key = await importKey(keyBytes); const nonce = randomBytes(NONCE_BYTES); const additionalData = new TextEncoder().encode('gone:v1');
    const ctBuf = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce, additionalData }, key, plaintextBytes); const ct = new Uint8Array(ctBuf);
    return { nonce, ciphertext: ct };
  }
  function exportKeyB64(k) { return b64urlEncode(k); }
  function importKeyB64(k) { const b = b64urlDecode(k); if (b.length !== KEY_BYTES) throw new Error('invalid key'); return b; }
  window.goneCrypto = { version: VERSION, generateKey, exportKeyB64, importKeyB64, encrypt, b64urlEncode, b64urlDecode };
  console.log('Gone crypto module loaded');
})();
