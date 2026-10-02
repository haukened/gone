'use strict';

// AES-256-GCM v1 primitives: 32-byte keys, fresh 12-byte nonces, and the
// static AAD "gone:v1". Exposed as window.goneCrypto.
(function cryptoScaffold() {
  const VERSION = 0x01;
  const KEY_BYTES = 32;
  const NONCE_BYTES = 12;
  const AAD = 'gone:v1';
  const MAX_FRAGMENT_CHARS = 512;
  const MAX_VERSION = 255;
  const B64URL_RE = /^[A-Za-z0-9_-]*$/;
  const FRAGMENT_RE = /^v([1-9][0-9]{0,2}):([A-Za-z0-9_-]+)$/;
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

  // b64urlDecode accepts only canonical unpadded base64url (docs/protocol.md
  // section 4): URL alphabet, no padding, zero trailing bits.
  function b64urlDecode(s) {
    if (typeof s !== 'string' || !B64URL_RE.test(s) || s.length % 4 === 1) {
      throw new Error('invalid base64url');
    }
    let norm = s.replace(/-/g, '+').replace(/_/g, '/');
    while (norm.length % 4) {
      norm += '=';
    }
    const bin = atob(norm);
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) {
      out[i] = bin.charCodeAt(i);
    }
    if (b64urlEncode(out) !== s) throw new Error('invalid base64url');
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

  function fragmentError(code) {
    const e = new Error(code);
    e.code = code;
    return e;
  }

  function fragmentKey(keyB64) {
    try {
      return importKeyB64(keyB64);
    } catch {
      throw fragmentError('invalid_fragment');
    }
  }

  // parseFragment parses a URL fragment without its leading "#" as
  // "v<version>:<key>" (docs/protocol.md section 7.2). It returns
  // {version, key} or throws an Error whose code is "invalid_fragment" or
  // "unsupported_version".
  function parseFragment(s) {
    const m = typeof s === 'string' && s.length <= MAX_FRAGMENT_CHARS ? FRAGMENT_RE.exec(s) : null;
    const version = m ? Number(m[1]) : 0;
    if (!m || version > MAX_VERSION) throw fragmentError('invalid_fragment');
    if (version !== VERSION) throw fragmentError('unsupported_version');
    return { version: version, key: fragmentKey(m[2]) };
  }

  window.goneCrypto = Object.freeze({
    version: VERSION,
    parseFragment: parseFragment,
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
