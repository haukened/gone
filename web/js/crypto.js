'use strict';

// Protocol primitives (docs/protocol.md section 4). v1 is AES-256-GCM with
// a 32-byte link key, a fresh 12-byte nonce, and the static AAD "gone:v1".
// v2 adds a passphrase: PBKDF2-SHA-256 and HKDF-SHA-256 combine it with the
// link key, and a 21-byte KDF header is prefixed to the ciphertext and bound
// into the AAD. Exposed as window.goneCrypto.
(function cryptoScaffold() {
  const VERSION = 0x01;
  const VERSION_V2 = 0x02;
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

  function gcmParams(nonce, aad) {
    return { name: 'AES-GCM', iv: nonce, additionalData: aad || new TextEncoder().encode(AAD) };
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

  function codedError(code) {
    const e = new Error(code);
    e.code = code;
    return e;
  }

  // v2Inputs groups the protocol v2 wire constants with the passphrase and
  // header rules, returning {headerBytes, saltBytes, iterations,
  // passphraseBytes, buildHeader, parseHeader}.
  function v2Inputs() {
    const TAG_BYTES = 16;
    const KDF_PBKDF2_SHA256 = 0x01;
    const SALT_BYTES = 16;
    const V2_HEADER_BYTES = 21;
    const V2_ITERATIONS = 600000;
    const V2_MIN_ITERATIONS = 600000;
    const V2_MAX_ITERATIONS = 5000000;
    const MAX_PASSPHRASE_BYTES = 1024;

    // wellFormed reports whether s has no lone UTF-16 surrogates, matching
    // the Go reader's "valid UTF-8" rule; encodeURIComponent throws on them.
    function wellFormed(s) {
      try {
        encodeURIComponent(s);
        return true;
      } catch {
        return false;
      }
    }

    // passphraseBytes applies the v2 passphrase rule: UTF-8 of the NFC form,
    // non-empty, at most 1024 bytes, never trimmed. The caller wipes the
    // result.
    function passphraseBytes(passphrase) {
      if (typeof passphrase !== 'string' || !wellFormed(passphrase)) throw codedError('invalid_passphrase');
      const p = new TextEncoder().encode(passphrase.normalize('NFC'));
      if (p.length === 0 || p.length > MAX_PASSPHRASE_BYTES) {
        p.fill(0);
        throw codedError('invalid_passphrase');
      }
      return p;
    }

    // buildV2Header encodes kdf_id || iterations (uint32 BE) || salt.
    function buildV2Header(iterations, salt) {
      const hdr = new Uint8Array(V2_HEADER_BYTES);
      hdr[0] = KDF_PBKDF2_SHA256;
      new DataView(hdr.buffer).setUint32(1, iterations);
      hdr.set(salt, 5);
      return hdr;
    }

    // parseV2Header bounds-checks the header at the start of a v2 blob and
    // throws code "malformed" when retrying with another passphrase cannot
    // help.
    function parseV2Header(blob) {
      if (!(blob instanceof Uint8Array) || blob.length < V2_HEADER_BYTES + TAG_BYTES || blob[0] !== KDF_PBKDF2_SHA256) {
        throw codedError('malformed');
      }
      const iterations = new DataView(blob.buffer, blob.byteOffset, V2_HEADER_BYTES).getUint32(1);
      if (iterations < V2_MIN_ITERATIONS || iterations > V2_MAX_ITERATIONS) throw codedError('malformed');
      return { iterations: iterations, salt: blob.subarray(5, V2_HEADER_BYTES), raw: blob.subarray(0, V2_HEADER_BYTES) };
    }

    return {
      headerBytes: V2_HEADER_BYTES, saltBytes: SALT_BYTES, iterations: V2_ITERATIONS,
      passphraseBytes: passphraseBytes, buildHeader: buildV2Header, parseHeader: parseV2Header
    };
  }

  // v2Codec seals and opens protocol v2 (passphrase) blobs using the rules
  // from v2Inputs, returning {encryptV2, decryptV2, headerBytes}.
  function v2Codec(inputs) {
    const AAD_V2 = 'gone:v2';
    const HKDF_INFO_V2 = 'gone:v2 aead key';

    // concat joins byte arrays into a fresh Uint8Array.
    function concat(a, b) {
      const out = new Uint8Array(a.length + b.length);
      out.set(a);
      out.set(b, a.length);
      return out;
    }

    // deriveV2Key turns the link key and normalized passphrase into the
    // non-extractable AES-256-GCM key. PBKDF2-SHA-256 gives pw, then
    // HKDF-SHA-256 over linkKey || pw. Intermediates are wiped; importKey
    // copies its input, so wiping after the call is safe.
    async function deriveV2Key(linkKey, p, salt, iterations, usage) {
      const subtle = crypto.subtle;
      const pbkdf = await subtle.importKey('raw', p, 'PBKDF2', false, ['deriveBits']);
      const pwParams = { name: 'PBKDF2', hash: 'SHA-256', salt: salt, iterations: iterations };
      const pw = new Uint8Array(await subtle.deriveBits(pwParams, pbkdf, KEY_BYTES * 8));
      const ikm = concat(linkKey, pw);
      pw.fill(0);
      const hkdf = await subtle.importKey('raw', ikm, 'HKDF', false, ['deriveKey']);
      ikm.fill(0);
      const info = new TextEncoder().encode(HKDF_INFO_V2);
      const hkdfParams = { name: 'HKDF', hash: 'SHA-256', salt: new Uint8Array(0), info: info };
      return subtle.deriveKey(hkdfParams, hkdf, { name: 'AES-GCM', length: 256 }, false, [usage]);
    }

    function v2AAD(hdr) {
      return concat(new TextEncoder().encode(AAD_V2), hdr);
    }

    // encryptV2 seals data under protocol v2 with a fresh salt (drawn first)
    // and nonce. It returns {nonce, ciphertext} where ciphertext is the full
    // blob hdr || ct || tag, or throws code "invalid_passphrase".
    async function encryptV2(data, keyBytes, passphrase) {
      assertKey(keyBytes);
      const plaintextBytes = toBytes(data);
      const p = inputs.passphraseBytes(passphrase);
      try {
        const salt = randomBytes(inputs.saltBytes);
        const hdr = inputs.buildHeader(inputs.iterations, salt);
        const key = await deriveV2Key(keyBytes, p, salt, inputs.iterations, 'encrypt');
        const nonce = randomBytes(NONCE_BYTES);
        const ct = await crypto.subtle.encrypt(gcmParams(nonce, v2AAD(hdr)), key, plaintextBytes);
        return { nonce: nonce, ciphertext: concat(hdr, new Uint8Array(ct)) };
      } finally {
        p.fill(0);
      }
    }

    // decryptV2 opens a v2 blob. It throws code "malformed" for a structurally
    // bad header (not retryable), "invalid_passphrase" for an unusable
    // passphrase, and "decrypt" for every authentication failure, including
    // a wrong passphrase (retryable).
    async function decryptV2(blob, nonce, keyBytes, passphrase) {
      if (!(keyBytes instanceof Uint8Array) || keyBytes.length !== KEY_BYTES || !nonce || nonce.length !== NONCE_BYTES) {
        throw codedError('decrypt');
      }
      const hdr = inputs.parseHeader(blob);
      const p = inputs.passphraseBytes(passphrase);
      let pt = null;
      try {
        const key = await deriveV2Key(keyBytes, p, hdr.salt, hdr.iterations, 'decrypt');
        pt = await crypto.subtle.decrypt(gcmParams(nonce, v2AAD(hdr.raw)), key, blob.subarray(inputs.headerBytes));
      } catch {
        pt = null;
      } finally {
        p.fill(0);
      }
      if (!pt) throw codedError('decrypt');
      return new Uint8Array(pt);
    }

    return { encryptV2: encryptV2, decryptV2: decryptV2, headerBytes: inputs.headerBytes };
  }

  const v2 = v2Codec(v2Inputs());

  function fragmentKey(keyB64) {
    try {
      return importKeyB64(keyB64);
    } catch {
      throw codedError('invalid_fragment');
    }
  }

  // parseFragment parses a URL fragment without its leading "#" as
  // "v<version>:<key>" (docs/protocol.md section 7.2); v1 and v2 share the
  // same 32-byte link key payload. It returns
  // {version, key} or throws an Error whose code is "invalid_fragment" or
  // "unsupported_version".
  function parseFragment(s) {
    const m = typeof s === 'string' && s.length <= MAX_FRAGMENT_CHARS ? FRAGMENT_RE.exec(s) : null;
    const version = m ? Number(m[1]) : 0;
    if (!m || version > MAX_VERSION) throw codedError('invalid_fragment');
    if (version !== VERSION && version !== VERSION_V2) throw codedError('unsupported_version');
    return { version: version, key: fragmentKey(m[2]) };
  }

  window.goneCrypto = Object.freeze({
    version: VERSION,
    versionV2: VERSION_V2,
    versions: Object.freeze([VERSION, VERSION_V2]),
    v2HeaderBytes: v2.headerBytes,
    parseFragment: parseFragment,
    generateKey: generateKey,
    exportKeyB64: exportKeyB64,
    importKeyB64: importKeyB64,
    encrypt: encrypt,
    decrypt: decrypt,
    encryptV2: v2.encryptV2,
    decryptV2: v2.decryptV2,
    b64urlEncode: b64urlEncode,
    b64urlDecode: b64urlDecode
  });
  console.log('Gone crypto module loaded');
})();
