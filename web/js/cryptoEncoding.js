'use strict';

// Base64url encoding for the Gone browser protocol. Exposed as
// window.goneCryptoEncoding for crypto.js assembly.
(function cryptoEncodingModule() {
  if (window.goneCryptoEncoding) return;
  const B64URL_RE = /^[A-Za-z0-9_-]*$/;

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

  window.goneCryptoEncoding = Object.freeze({ b64urlEncode: b64urlEncode, b64urlDecode: b64urlDecode });
})();
