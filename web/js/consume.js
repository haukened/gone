'use strict';

// Secret consumption flow: claim (streamed GET) -> decrypt -> decode -> ack (DELETE).
// The secret is only deleted from the server once the full payload has been
// received and authenticated by AES-GCM, so interrupted downloads can retry.
// Network/crypto live in consumeApi.js; DOM rendering in consumeView.js.
(function consumeFlow() {
  const util = window.goneUtil;
  const api = window.goneConsumeApi;
  const view = window.goneConsumeView;
  if (!util || !util.allPresent([window.goneCrypto, window.goneEnvelope, api, view]) || !view.present) return;

  const MALFORMED = 'This secret\u2019s contents are malformed; ask the sender to resend it.';
  const PREVIEW_TEXT = 'This is a preview of a decrypted secret. Customize via ?text=...';

  function decodeEnvelope(plaintext) {
    try {
      return window.goneEnvelope.decode(plaintext);
    } catch (e) {
      console.error('[gone] invalid envelope', e);
      throw api.FetchError(MALFORMED, false);
    }
  }

  async function run(keyB64) {
    const t0 = performance.now();
    const claim = { token: '' };
    view.setStatus('Retrieving\u2026');
    const fetched = await api.fetchWithRetry(claim, view.setProgress);
    util.logTiming('consume_fetch', t0, performance.now());
    view.setStatus('Decrypting\u2026');
    const plaintext = await api.decrypt(fetched.resp, fetched.body, keyB64);
    view.keepPlaintext(plaintext);
    view.showDecoded(decodeEnvelope(plaintext));
    util.logTiming('consume_total', t0, performance.now());
    view.showAckResult(await api.acknowledge(claim));
  }

  function handlePreview(params) {
    view.showMessage(params.get('text') || PREVIEW_TEXT);
    view.setStatus('Decrypted (preview)');
  }

  function parseFragment(hash) {
    const m = /^#v(\d+):([A-Za-z0-9_-]{10,})$/.exec(hash || '');
    if (!m) return null;
    return { version: parseInt(m[1], 10), keyB64: m[2] };
  }

  // keyProblem returns a status message when the fragment is unusable.
  function keyProblem(frag) {
    if (!frag) return 'Missing or invalid key fragment. Cannot decrypt.';
    if (frag.version !== window.goneCrypto.version) return 'Unsupported version';
    return '';
  }

  // registerFromPath pins the API endpoint to the id in the page URL.
  // Returns false when the id is invalid.
  function registerFromPath() {
    const parts = location.pathname.split('/');
    try {
      api.registerEndpoint(parts[parts.length - 1]);
      return true;
    } catch (_) {
      return false;
    }
  }

  function fail(e) {
    const known = api.isFetchError(e);
    console.error('[gone] consume error', known ? e.message : e);
    view.hideProgress();
    view.setStatus(known ? e.message : 'Unexpected error');
  }

  function start() {
    const params = new URLSearchParams(location.search);
    if (params.get('preview') === 'secret') {
      handlePreview(params);
      return;
    }
    const frag = parseFragment(location.hash);
    const problem = keyProblem(frag);
    if (problem) {
      view.setStatus(problem);
      return;
    }
    if (!registerFromPath()) {
      view.setStatus('Invalid secret id');
      return;
    }
    window.addEventListener('beforeunload', view.guardUnload);
    window.addEventListener('pagehide', view.cleanup);
    run(frag.keyB64).catch(fail);
  }

  start();
})();
