'use strict';

// Protocol v2 key derivation helpers.
(function cryptoV2KeyModule() {
  if (window.goneCryptoV2Key || !window.goneCryptoCore) return;
  const core = window.goneCryptoCore;
  const HKDF_INFO_V2 = 'gone:v2 aead key';

  function concat(a, b) {
    const out = new Uint8Array(a.length + b.length);
    out.set(a);
    out.set(b, a.length);
    return out;
  }

  // deriveKey turns the link key and normalized passphrase into the
  // non-extractable AES-256-GCM key. Intermediates are wiped after use.
  async function deriveKey(linkKey, p, salt, iterations, usage) {
    const subtle = crypto.subtle;
    const pbkdf = await subtle.importKey('raw', p, 'PBKDF2', false, ['deriveBits']);
    const pwParams = { name: 'PBKDF2', hash: 'SHA-256', salt: salt, iterations: iterations };
    const pw = new Uint8Array(await subtle.deriveBits(pwParams, pbkdf, core.KEY_BYTES * 8));
    const ikm = concat(linkKey, pw);
    pw.fill(0);
    const hkdf = await subtle.importKey('raw', ikm, 'HKDF', false, ['deriveKey']);
    ikm.fill(0);
    const info = new TextEncoder().encode(HKDF_INFO_V2);
    const hkdfParams = { name: 'HKDF', hash: 'SHA-256', salt: new Uint8Array(0), info: info };
    return subtle.deriveKey(hkdfParams, hkdf, { name: 'AES-GCM', length: 256 }, false, [usage]);
  }

  window.goneCryptoV2Key = Object.freeze({ concat: concat, deriveKey: deriveKey });
})();
