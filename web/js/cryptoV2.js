'use strict';

// Protocol v2 passphrase encryption/decryption.
(function cryptoV2Module() {
  if (window.goneCryptoV2 || !window.goneCryptoCore || !window.goneCryptoV2Inputs || !window.goneCryptoV2Key) return;
  const core = window.goneCryptoCore;
  const inputs = window.goneCryptoV2Inputs;
  const keying = window.goneCryptoV2Key;
  const AAD_V2 = 'gone:v2';

  function v2AAD(hdr) {
    return keying.concat(new TextEncoder().encode(AAD_V2), hdr);
  }

  // encryptV2 seals data under protocol v2 with a fresh salt and nonce.
  async function encryptV2(data, keyBytes, passphrase) {
    core.assertKey(keyBytes);
    const plaintextBytes = core.toBytes(data);
    const p = inputs.passphraseBytes(passphrase);
    try {
      const salt = core.randomBytes(inputs.saltBytes);
      const hdr = inputs.buildHeader(inputs.iterations, salt);
      const key = await keying.deriveKey(keyBytes, p, salt, inputs.iterations, 'encrypt');
      const nonce = core.randomBytes(core.NONCE_BYTES);
      const ct = await crypto.subtle.encrypt(core.gcmParams(nonce, v2AAD(hdr)), key, plaintextBytes);
      return { nonce: nonce, ciphertext: keying.concat(hdr, new Uint8Array(ct)) };
    } finally {
      p.fill(0);
    }
  }

  // decryptV2 opens a v2 blob and maps authentication failures to "decrypt".
  async function decryptV2(blob, nonce, keyBytes, passphrase) {
    if (!(keyBytes instanceof Uint8Array) || keyBytes.length !== core.KEY_BYTES || !nonce || nonce.length !== core.NONCE_BYTES) {
      throw core.codedError('decrypt');
    }
    const hdr = inputs.parseHeader(blob);
    const p = inputs.passphraseBytes(passphrase);
    let pt = null;
    try {
      const key = await keying.deriveKey(keyBytes, p, hdr.salt, hdr.iterations, 'decrypt');
      pt = await crypto.subtle.decrypt(core.gcmParams(nonce, v2AAD(hdr.raw)), key, blob.subarray(inputs.headerBytes));
    } catch {
      pt = null;
    } finally {
      p.fill(0);
    }
    if (!pt) throw core.codedError('decrypt');
    return new Uint8Array(pt);
  }

  window.goneCryptoV2 = Object.freeze({ encryptV2: encryptV2, decryptV2: decryptV2, headerBytes: inputs.headerBytes });
})();
