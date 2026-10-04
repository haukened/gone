'use strict';

// Encrypt-and-upload side of the create flow: builds the envelope, encrypts it
// under a fresh key, POSTs the ciphertext to the fixed same-origin API path,
// and builds the share link. Requires window.goneCrypto, window.goneEnvelope
// and window.goneUtil. Exposed as window.goneUpload.
(function uploadModule() {
  if (window.goneUpload || !window.goneCrypto || !window.goneEnvelope || !window.goneUtil) return;
  const gc = window.goneCrypto;
  const util = window.goneUtil;

  const UPLOAD_PATH = '/api/secret';
  const NETWORK_ERROR = 'Network error uploading secret';
  const TRANSPORT_ERROR_RE = /network|abort|timed/;
  const SECRET_ID_RE = /^[0-9a-f]{32}$/;
  const MANAGE_TOKEN_RE = /^[A-Za-z0-9_-]{43}$/;
  const UPLOAD_ERRORS = new Map([
    [400, 'The server rejected the secret'],
    [413, 'Secret too large for this server'],
    [429, 'Slow down: too many requests. Please wait and retry.'],
    [503, 'The server is busy right now. Wait a moment, then try again.']
  ]);

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

  function parseJSON(text) {
    try {
      return JSON.parse(text);
    } catch {
      return null;
    }
  }

  // xhrUpload posts the ciphertext with XMLHttpRequest because fetch has no
  // upload progress events. The URL is a fixed same-origin path.
  function xhrUpload(ciphertext, headers, onProgress) {
    return new Promise(function (resolve, reject) {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', UPLOAD_PATH);
      Object.keys(headers).forEach(function (k) { xhr.setRequestHeader(k, headers[k]); });
      xhr.upload.onprogress = function (e) {
        if (e.lengthComputable) onProgress(e.loaded, e.total);
      };
      xhr.onload = function () { resolve({ status: xhr.status, json: parseJSON(xhr.responseText) }); };
      xhr.onerror = function () { reject(new Error('network error')); };
      xhr.onabort = function () { reject(new Error('upload aborted')); };
      xhr.ontimeout = function () { reject(new Error('upload timed out')); };
      xhr.send(ciphertext);
    });
  }

  function uploadErrorMessage(status) {
    return UPLOAD_ERRORS.get(status) || 'Server error creating secret';
  }

  // upload sends the encrypted secret with the given TTL under protocol
  // version (default v1).
  //
  // Returns the server's JSON ({id, expires_at, manage_token}); rejects with a user-facing
  // message on a non-success status or malformed response.
  async function upload(encResult, ttl, onProgress, version) {
    const headers = {
      'X-Gone-Version': String(version || gc.version),
      'X-Gone-Nonce': gc.b64urlEncode(encResult.nonce),
      'X-Gone-TTL': ttl,
      'Content-Type': 'application/octet-stream'
    };
    onProgress(0, encResult.ciphertext.length);
    const t0 = performance.now();
    const res = await xhrUpload(encResult.ciphertext, headers, onProgress);
    util.logTiming('upload', t0, performance.now());
    if (res.status !== 201 && res.status !== 200) {
      console.error('[gone] server error', res.status);
      throw new Error(uploadErrorMessage(res.status));
    }
    if (!res.json || !res.json.id) throw new Error('Unexpected server response');
    return res.json;
  }

  // friendlyError maps transport failures to a generic network message and
  // passes server messages through.
  function friendlyError(e) {
    const msg = e && e.message;
    if (!msg || TRANSPORT_ERROR_RE.test(msg)) return NETWORK_ERROR;
    return msg;
  }

  // buildShareURL returns the link the recipient opens for protocol version
  // (default v1); the key travels only in the fragment, which browsers never
  // send to the server.
  function buildShareURL(id, keyBytes, version) {
    return `${location.origin}/secret/${id}#v${version || gc.version}:${gc.exportKeyB64(keyBytes)}`;
  }

  // buildManageURL returns the sender's private manage link, with the manage
  // token in the fragment so it never reaches server logs via the path.
  //
  // Returns '' when id or token is malformed, so the result view can omit
  // the manage section rather than show a broken link.
  function buildManageURL(id, token) {
    // Both patterns are anchored fixed-length character classes, so they run in linear time.
    if (!SECRET_ID_RE.test(String(id)) || !MANAGE_TOKEN_RE.test(String(token))) return ''; // nosemgrep
    return `${location.origin}/manage/${id}#${token}`;
  }

  window.goneUpload = Object.freeze({
    buildPlaintext: buildPlaintext,
    encryptSelection: encryptSelection,
    upload: upload,
    uploadErrorMessage: uploadErrorMessage,
    friendlyError: friendlyError,
    buildShareURL: buildShareURL,
    buildManageURL: buildManageURL
  });
})();
