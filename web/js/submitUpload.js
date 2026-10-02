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
  const UPLOAD_ERRORS = new Map([
    [400, 'The server rejected the secret'],
    [413, 'Secret too large for this server'],
    [429, 'Slow down: too many requests. Please wait and retry.']
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

  // encryptSelection encrypts message and files under a fresh key.
  //
  // Returns {keyBytes, encResult}; the caller must zero keyBytes when done.
  async function encryptSelection(message, files) {
    const t0 = performance.now();
    const plaintext = await buildPlaintext(message, files);
    const keyBytes = gc.generateKey();
    try {
      const encResult = await gc.encrypt(plaintext, keyBytes);
      util.logTiming('encrypt', t0, performance.now());
      return { keyBytes: keyBytes, encResult: encResult };
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

  // upload sends the encrypted secret with the given TTL.
  //
  // Returns the server's JSON ({id, expires_at}); rejects with a user-facing
  // message on a non-success status or malformed response.
  async function upload(encResult, ttl, onProgress) {
    const headers = {
      'X-Gone-Version': String(gc.version),
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

  // buildShareURL returns the link the recipient opens; the key travels only
  // in the fragment, which browsers never send to the server.
  function buildShareURL(id, keyBytes) {
    return `${location.origin}/secret/${id}#v${gc.version}:${gc.exportKeyB64(keyBytes)}`;
  }

  window.goneUpload = Object.freeze({
    buildPlaintext: buildPlaintext,
    encryptSelection: encryptSelection,
    upload: upload,
    uploadErrorMessage: uploadErrorMessage,
    friendlyError: friendlyError,
    buildShareURL: buildShareURL
  });
})();
