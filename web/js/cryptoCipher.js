'use strict';

// AES-GCM v1 encryption/decryption for the Gone browser protocol.
(function cryptoCipherModule() {
  if (window.goneCryptoCipher || !window.goneCryptoCore || !window.goneCryptoEncoding) return;
  const core = window.goneCryptoCore;
  const enc = window.goneCryptoEncoding;

  // encrypt seals data under keyBytes with a fresh random nonce.
  async function encrypt(data, keyBytes) {
    core.assertKey(keyBytes);
    const plaintextBytes = core.toBytes(data);
    const key = await core.importAesKey(keyBytes, 'encrypt');
    const nonce = core.randomBytes(core.NONCE_BYTES);
    const ctBuf = await crypto.subtle.encrypt(core.gcmParams(nonce), key, plaintextBytes);
    return { nonce: nonce, ciphertext: new Uint8Array(ctBuf) };
  }

  // decrypt opens ciphertext sealed by encrypt; it rejects if the key,
  // nonce, AAD, or ciphertext do not authenticate.
  async function decrypt(ciphertext, nonce, keyBytes) {
    core.assertKey(keyBytes);
    if (nonce.length !== core.NONCE_BYTES) throw new Error('invalid nonce length');
    const key = await core.importAesKey(keyBytes, 'decrypt');
    const pt = await crypto.subtle.decrypt(core.gcmParams(nonce), key, ciphertext);
    return new Uint8Array(pt);
  }

  function exportKeyB64(k) {
    return enc.b64urlEncode(k);
  }

  window.goneCryptoCipher = Object.freeze({ encrypt: encrypt, decrypt: decrypt, exportKeyB64: exportKeyB64 });
})();
