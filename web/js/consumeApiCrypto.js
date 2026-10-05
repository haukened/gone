'use strict';

// Decryption and protocol verification for consumed secrets.
(function consumeApiCryptoModule() {
  if (window.goneConsumeApiCrypto || !window.goneConsumeApiErrors || !window.goneCrypto) return;
  const errs = window.goneConsumeApiErrors;
  const PASSPHRASE_ERROR = 'That passphrase didn\u2019t work. The secret is already gone from the server, and this browser holds the only copy, still encrypted. Check the passphrase and try again here: closing or leaving this page destroys the secret for good.';
  const DAMAGED_ERROR = 'This secret\u2019s contents are damaged. Ask the sender to share it again.';
  const NONCE_BYTES = 12;

  function readNonce(resp, frag) {
    if (resp.headers.get('X-Gone-Version') !== String(frag.version)) throw errs.FetchError('Unsupported secret version', false);
    let nonce;
    try {
      nonce = window.goneCrypto.b64urlDecode(resp.headers.get('X-Gone-Nonce') || '');
    } catch {
      nonce = null;
    }
    if (!nonce || nonce.length !== NONCE_BYTES) throw errs.FetchError(errs.VERIFY_ERROR, false);
    return nonce;
  }

  async function decrypt(resp, ciphertext, frag) {
    let plaintext;
    try {
      const nonce = readNonce(resp, frag);
      plaintext = await window.goneCrypto.decrypt(ciphertext, nonce, frag.key);
    } catch (e) {
      throw errs.isFetchError(e) ? e : errs.FetchError(errs.VERIFY_ERROR, false);
    } finally {
      frag.key.fill(0);
      ciphertext.fill(0);
    }
    return plaintext;
  }

  function v2Failure(e) {
    if (e.code === 'decrypt' || e.code === 'invalid_passphrase') {
      const err = errs.FetchError(PASSPHRASE_ERROR, true);
      err.passphrase = true;
      return err;
    }
    if (errs.isFetchError(e)) return e;
    return errs.FetchError(e.code === 'malformed' ? DAMAGED_ERROR : errs.VERIFY_ERROR, false);
  }

  async function decryptV2(resp, ciphertext, frag, passphrase) {
    try {
      const nonce = readNonce(resp, frag);
      const plaintext = await window.goneCrypto.decryptV2(ciphertext, nonce, frag.key, passphrase);
      frag.key.fill(0);
      ciphertext.fill(0);
      return plaintext;
    } catch (e) {
      const err = v2Failure(e);
      if (!err.passphrase) {
        frag.key.fill(0);
        ciphertext.fill(0);
      }
      throw err;
    }
  }

  window.goneConsumeApiCrypto = Object.freeze({ decrypt: decrypt, decryptV2: decryptV2 });
})();
