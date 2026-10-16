'use strict';

// Transport side of the create flow: the XMLHttpRequest upload (fetch has no
// upload progress events) and the message keys for upload failures. Exposed
// as window.goneSubmitXhr.
(function submitXhrModule() {
  if (window.goneSubmitXhr) return;

  const UPLOAD_PATH = '/api/secret';
  const NETWORK_ERROR = 'js.upload.network';
  const TRANSPORT_ERROR_RE = /network|abort|timed/;
  const SERVER_ERROR = 'js.upload.server';
  const UPLOAD_ERRORS = new Map([
    [400, 'js.upload.rejected'],
    [403, 'js.upload.passRequired'],
    [413, 'js.upload.tooLarge'],
    [429, 'js.common.tooMany'],
    [503, 'js.common.busy']
  ]);

  function parseJSON(text) {
    try {
      return JSON.parse(text);
    } catch {
      return null;
    }
  }

  // xhrUpload sends the ciphertext with XMLHttpRequest because fetch has no
  // upload progress events. The URL is a fixed same-origin path.
  //
  // Returns {status, json}; rejects only on a transport failure.
  // target is {method, path} for a same-origin path chosen by the caller;
  // the default is POST /api/secret.
  function xhrUpload(ciphertext, headers, onProgress, target) {
    const t = target || { method: 'POST', path: UPLOAD_PATH };
    return new Promise(function (resolve, reject) {
      const xhr = new XMLHttpRequest();
      xhr.open(t.method, t.path);
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

  // uploadErrorMessage returns the message key for a failed upload status.
  function uploadErrorMessage(status) {
    return UPLOAD_ERRORS.get(status) || SERVER_ERROR;
  }

  // friendlyError returns the message key to show for an upload failure:
  // transport failures get the network message, keyed errors pass through,
  // anything else gets the generic server message.
  function friendlyError(e) {
    const msg = e && e.message;
    if (!msg || TRANSPORT_ERROR_RE.test(msg)) return NETWORK_ERROR;
    return msg.indexOf('js.') === 0 ? msg : SERVER_ERROR;
  }

  window.goneSubmitXhr = Object.freeze({
    SERVER_ERROR: SERVER_ERROR,
    send: xhrUpload,
    uploadErrorMessage: uploadErrorMessage,
    friendlyError: friendlyError
  });
})();
