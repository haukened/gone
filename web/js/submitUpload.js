'use strict';

// Encrypt-and-upload side of the create flow: builds the envelope, encrypts it
// under a fresh key, POSTs the ciphertext to the fixed same-origin API path,
// and builds the share link. Requires window.goneCrypto, window.goneEnvelope,
// window.goneUtil and window.goneSubmitXhr (submitXhr.js). Exposed as
// window.goneUpload.
(function uploadModule() {
  if (window.goneUpload || !window.goneCrypto || !window.goneEnvelope || !window.goneUtil || !window.goneSubmitXhr) return;
  const gc = window.goneCrypto;
  const util = window.goneUtil;
  const xhr = window.goneSubmitXhr;

  const SECRET_ID_RE = /^[0-9a-f]{32}$/;
  const SERVER_ERROR = xhr.SERVER_ERROR;
  const BAD_ID = 'js.upload.badId';

  // buildPlaintext reads the files and encodes them with message into an
  // envelope. Intermediate file buffers are zeroed.
  async function buildPlaintext(message, files) {
    const buffers = await Promise.all(files.map(function (f) { return f.arrayBuffer(); }));
    const parts = files.map(function (f, i) {
      return { name: f.name, type: f.type, bytes: new Uint8Array(buffers[i]) };
    });
    const plaintext = window.goneEnvelope.encode(message, parts);
    parts.forEach(function (p) { p.bytes.fill(0); });
    return plaintext;
  }

  // seal encrypts plaintext under keyBytes: protocol v2 when a passphrase is
  // given, v1 otherwise. Returns {nonce, ciphertext}.
  function seal(plaintext, keyBytes, passphrase) {
    return passphrase ? gc.encryptV2(plaintext, keyBytes, passphrase) : gc.encrypt(plaintext, keyBytes);
  }

  // encryptSelection encrypts message and files under a fresh key, adding the
  // optional passphrase (protocol v2) when it is non-empty.
  //
  // Returns {keyBytes, encResult, version}; the caller must zero keyBytes when done.
  async function encryptSelection(message, files, passphrase) {
    const t0 = performance.now();
    const plaintext = await buildPlaintext(message, files);
    const keyBytes = gc.generateKey();
    try {
      const encResult = await seal(plaintext, keyBytes, passphrase);
      util.logTiming('encrypt', t0, performance.now());
      return { keyBytes: keyBytes, encResult: encResult, version: passphrase ? gc.versionV2 : gc.version };
    } finally {
      plaintext.fill(0);
    }
  }

  // upload sends the encrypted secret with the given TTL under protocol
  // version (default v1).
  //
  // Returns the server's JSON ({id, expires_at, manage_token}); rejects with a message
  // key on a non-success status or malformed response.
  async function upload(encResult, ttl, onProgress, version) {
    const headers = {
      'X-Gone-Version': String(version || gc.version),
      'X-Gone-Nonce': gc.b64urlEncode(encResult.nonce),
      'X-Gone-TTL': ttl,
      'Content-Type': 'application/octet-stream'
    };
    onProgress(0, encResult.ciphertext.length);
    const t0 = performance.now();
    const res = await xhr.send(encResult.ciphertext, headers, onProgress);
    util.logTiming('upload', t0, performance.now());
    if (res.status !== 201 && res.status !== 200) {
      console.error('[gone] server error', res.status);
      throw new Error(xhr.uploadErrorMessage(res.status));
    }
    if (!res.json || !res.json.id) throw new Error(SERVER_ERROR);
    // Checked here, before the caller wipes the form, so the sender keeps
    // their text when the server returns an id that can't make a link.
    if (!SECRET_ID_RE.test(String(res.json.id))) throw new Error(BAD_ID); // nosemgrep
    return res.json;
  }

  // buildShareURL returns the link the recipient opens for protocol version
  // (default v1); the key travels only in the fragment, which browsers never
  // send to the server.
  //
  // Returns '' when the server-supplied id is malformed, so a bad id is never
  // put into a link or QR code.
  function buildShareURL(id, keyBytes, version) {
    // Anchored fixed-length character class, so it runs in linear time.
    if (!SECRET_ID_RE.test(String(id))) return ''; // nosemgrep
    return `${location.origin}/secret/${id}#v${version || gc.version}:${gc.exportKeyB64(keyBytes)}`;
  }

  // buildManageURL returns the sender's private manage link, with the manage
  // token in the fragment so it never reaches server logs via the path.
  //
  // Returns '' when id or token is malformed, so the result view can omit
  // the manage section rather than show a broken link.
  function buildManageURL(id, token) {
    // The ID pattern is an anchored fixed-length character class, so it runs in linear time.
    if (!SECRET_ID_RE.test(String(id)) || !util.isToken(token)) return ''; // nosemgrep
    return `${location.origin}/manage/${id}#${token}`;
  }

  window.goneUpload = Object.freeze({
    buildPlaintext: buildPlaintext,
    encryptSelection: encryptSelection,
    upload: upload,
    send: xhr.send,
    uploadErrorMessage: xhr.uploadErrorMessage,
    friendlyError: xhr.friendlyError,
    buildShareURL: buildShareURL,
    buildManageURL: buildManageURL
  });
})();
