'use strict';

// Public Gone crypto namespace assembled from the small protocol modules.
(function cryptoModule() {
  if (window.goneCrypto) return;
  if (!window.goneCryptoEncoding || !window.goneCryptoCore || !window.goneCryptoCipher || !window.goneCryptoV2) return;
  const enc = window.goneCryptoEncoding;
  const core = window.goneCryptoCore;
  const cipher = window.goneCryptoCipher;
  const v2 = window.goneCryptoV2;
  const MAX_FRAGMENT_CHARS = 512;
  const MAX_VERSION = 255;
  const FRAGMENT_RE = /^v([1-9][0-9]{0,2}):([A-Za-z0-9_-]+)$/;

  function fragmentMatch(s) {
    return typeof s === 'string' && s.length <= MAX_FRAGMENT_CHARS ? FRAGMENT_RE.exec(s) : null;
  }

  function validVersion(version) {
    if (version > MAX_VERSION) throw core.codedError('invalid_fragment');
    if (version !== core.VERSION && version !== core.VERSION_V2) throw core.codedError('unsupported_version');
  }

  function fragmentKey(keyB64) {
    try {
      return core.importKeyB64(keyB64);
    } catch {
      throw core.codedError('invalid_fragment');
    }
  }

  // parseFragment parses "v<version>:<key>" into {version, key}.
  function parseFragment(s) {
    const m = fragmentMatch(s);
    if (!m) throw core.codedError('invalid_fragment');
    const version = Number(m[1]);
    validVersion(version);
    return { version: version, key: fragmentKey(m[2]) };
  }

  window.goneCrypto = Object.freeze({
    version: core.VERSION,
    versionV2: core.VERSION_V2,
    versions: Object.freeze([core.VERSION, core.VERSION_V2]),
    v2HeaderBytes: v2.headerBytes,
    parseFragment: parseFragment,
    generateKey: core.generateKey,
    exportKeyB64: cipher.exportKeyB64,
    importKeyB64: core.importKeyB64,
    encrypt: cipher.encrypt,
    decrypt: cipher.decrypt,
    encryptV2: v2.encryptV2,
    decryptV2: v2.decryptV2,
    b64urlEncode: enc.b64urlEncode,
    b64urlDecode: enc.b64urlDecode
  });
  console.log('Gone crypto module loaded');
})();
