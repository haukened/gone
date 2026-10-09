'use strict';

// The reply page, /reply/{id}#v3:<pubkey>.<fill>: the person who holds the
// secret answers a request. The fragment is validated before any request,
// then a read-only check confirms the request is still open, so nobody types
// a secret into a dead link. It then provides window.goneSubmitTarget, which
// the shared send-form scripts (loaded after this one) use to encrypt to the
// requester's public key (protocol v3) and PUT the single reply.
(function replyFlow() {
  if (window.goneSubmitTarget) return;
  const util = window.goneUtil;
  const v3 = window.goneCryptoV3;
  const uploader = window.goneUpload;
  if (!util || !util.allPresent([v3, uploader, window.goneCrypto]) || !document.getElementById('view-sent')) return;

  const ID_RE = /^[0-9a-f]{32}$/;
  const VIEWS = ['check', 'compose', 'sent', 'gone'];
  const BAD_LINK = 'This request link isn\u2019t complete. Check that you copied all of it, including everything after the #.';
  const NEWER = 'This link was made by a newer version of Gone and can\u2019t be answered here.';
  const BAD_KEY = 'This request link is damaged, so nothing can be encrypted for it. Ask them for a new link.';
  const NETWORK_ERROR = 'Couldn\u2019t reach the server. Check your connection, then try again.';
  const CHECK_ERRORS = new Map([
    [400, BAD_LINK],
    [429, 'Too many requests right now. Wait a moment, then try again.'],
    [503, 'The server is busy right now. Wait a moment, then try again.']
  ]);
  const TITLES = { compose: 'Gone \u00b7 Send a secret', sent: 'Gone \u00b7 Sent', gone: 'Gone \u00b7 This request is gone' };

  const byId = function (id) { return document.getElementById(id); };

  function switchTo(name) {
    VIEWS.forEach(function (v) {
      const node = byId(v === 'compose' ? 'compose' : `view-${v}`);
      if (node) node.hidden = v !== name;
    });
    if (TITLES[name]) document.title = TITLES[name];
    const heading = byId(`${name}-heading`);
    if (heading) heading.focus();
  }

  function showCheckError(message, retryable) {
    util.setText(byId('check-status'), '');
    util.setText(byId('check-error-text'), message);
    byId('check-error').hidden = false;
    byId('check-retry').hidden = !retryable;
  }

  // readLink validates the id and fragment. Returns {id, frag} or {problem}.
  function readLink() {
    const parts = location.pathname.split('/');
    const id = parts[parts.length - 1];
    if (!ID_RE.test(id)) return { problem: BAD_LINK };
    try {
      return { id: id, frag: v3.parseReplyFragment(String(location.hash).replace(/^#/, '')) };
    } catch (e) {
      return { problem: e.code === 'unsupported_version' ? NEWER : BAD_LINK };
    }
  }

  function setExpiry(value) {
    const node = byId('reply-expires');
    const when = new Date(value);
    if (!node || Number.isNaN(when.getTime())) return;
    node.textContent = when.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
    node.setAttribute('datetime', when.toISOString());
  }

  // checkOpen asks whether the request can still be answered.
  async function checkOpen(link) {
    util.setText(byId('check-status'), 'Checking\u2026');
    byId('check-error').hidden = true;
    let resp;
    try {
      resp = await fetch(new URL(`/api/request/${link.id}`, location.origin).href, {
        headers: { 'X-Gone-Fill': link.frag.fill }, mode: 'same-origin', credentials: 'same-origin', redirect: 'error', cache: 'no-store'
      });
    } catch {
      showCheckError(NETWORK_ERROR, true);
      return;
    }
    if (resp.status === 404) {
      switchTo('gone');
      return;
    }
    if (resp.status !== 200) {
      showCheckError(CHECK_ERRORS.get(resp.status) || 'The server had a problem. Try again in a moment.', resp.status !== 400);
      return;
    }
    const body = await resp.json().catch(function () { return {}; });
    setExpiry(body.expires_at);
    switchTo('compose');
  }

  // target encrypts the form's message and files to the requester's key and
  // sends them as the request's one reply.
  function makeTarget(link) {
    return {
      encrypt: async function (message, files) {
        const plaintext = await uploader.buildPlaintext(message, files);
        try {
          return { encResult: await v3.encryptV3(plaintext, link.frag.publicKey) };
        } catch (e) {
          if (e && e.code === 'invalid_key') e.userMessage = BAD_KEY;
          throw e;
        } finally {
          plaintext.fill(0);
        }
      },
      upload: async function (enc, onProgress) {
        const headers = {
          'X-Gone-Version': String(v3.version),
          'X-Gone-Nonce': window.goneCrypto.b64urlEncode(enc.encResult.nonce),
          'X-Gone-Fill': link.frag.fill,
          'Content-Type': 'application/octet-stream'
        };
        onProgress(0, enc.encResult.ciphertext.length);
        const res = await uploader.send(enc.encResult.ciphertext, headers, onProgress, { method: 'PUT', path: `/api/request/${link.id}/reply` });
        if (res.status === 404) switchTo('gone');
        if (res.status !== 201) throw new Error(res.status === 404 ? 'This request is gone.' : uploader.uploadErrorMessage(res.status));
        return res.json || {};
      },
      show: function () { switchTo('sent'); },
      wipe: function (enc) { enc.encResult.ciphertext.fill(0); }
    };
  }

  function start() {
    const link = readLink();
    if (link.problem) {
      showCheckError(link.problem, false);
      return;
    }
    window.goneSubmitTarget = makeTarget(link);
    byId('check-retry').addEventListener('click', function () { checkOpen(link); });
    checkOpen(link);
  }

  start();
})();
