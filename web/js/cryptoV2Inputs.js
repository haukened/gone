'use strict';

// Protocol v2 KDF header and passphrase validation rules.
(function cryptoV2InputsModule() {
  if (window.goneCryptoV2Inputs || !window.goneCryptoCore) return;
  const core = window.goneCryptoCore;
  const TAG_BYTES = 16;
  const KDF_PBKDF2_SHA256 = 0x01;
  const SALT_BYTES = 16;
  const V2_HEADER_BYTES = 21;
  const V2_ITERATIONS = 600000;
  const V2_MIN_ITERATIONS = 600000;
  const V2_MAX_ITERATIONS = 5000000;
  const MAX_PASSPHRASE_BYTES = 1024;

  function wellFormed(s) {
    try {
      encodeURIComponent(s);
      return true;
    } catch {
      return false;
    }
  }

  function passphraseBytes(passphrase) {
    if (typeof passphrase !== 'string' || !wellFormed(passphrase)) throw core.codedError('invalid_passphrase');
    const p = new TextEncoder().encode(passphrase.normalize('NFC'));
    if (p.length === 0 || p.length > MAX_PASSPHRASE_BYTES) {
      p.fill(0);
      throw core.codedError('invalid_passphrase');
    }
    return p;
  }

  function buildHeader(iterations, salt) {
    const hdr = new Uint8Array(V2_HEADER_BYTES);
    hdr[0] = KDF_PBKDF2_SHA256;
    new DataView(hdr.buffer).setUint32(1, iterations);
    hdr.set(salt, 5);
    return hdr;
  }

  function parseHeader(blob) {
    if (!(blob instanceof Uint8Array) || blob.length < V2_HEADER_BYTES + TAG_BYTES || blob[0] !== KDF_PBKDF2_SHA256) {
      throw core.codedError('malformed');
    }
    const iterations = new DataView(blob.buffer, blob.byteOffset, V2_HEADER_BYTES).getUint32(1);
    if (iterations < V2_MIN_ITERATIONS || iterations > V2_MAX_ITERATIONS) throw core.codedError('malformed');
    return { iterations: iterations, salt: blob.subarray(5, V2_HEADER_BYTES), raw: blob.subarray(0, V2_HEADER_BYTES) };
  }

  window.goneCryptoV2Inputs = Object.freeze({ headerBytes: V2_HEADER_BYTES, saltBytes: SALT_BYTES, iterations: V2_ITERATIONS, passphraseBytes: passphraseBytes, buildHeader: buildHeader, parseHeader: parseHeader });
})();
