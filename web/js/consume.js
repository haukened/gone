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
  const BAD_FRAGMENT = 'Missing or invalid key fragment. Cannot decrypt.';
  const FRAGMENT_PROBLEMS = new Map([['unsupported_version', 'Unsupported version']]);
  const PREVIEW_TEXT = 'This is a preview of a decrypted secret. Customize via ?text=...';

  function decodeEnvelope(plaintext) {
    try {
      return window.goneEnvelope.decode(plaintext);
    } catch (e) {
      console.error('[gone] invalid envelope', e);
      throw api.FetchError(MALFORMED, false);
    }
  }

  async function run(frag) {
    const t0 = performance.now();
    const claim = { token: '' };
    view.setStatus('Retrieving\u2026');
    const fetched = await api.fetchWithRetry(claim, view.setProgress);
    util.logTiming('consume_fetch', t0, performance.now());
    view.setStatus('Decrypting\u2026');
    const plaintext = await api.decrypt(fetched.resp, fetched.body, frag);
    view.keepPlaintext(plaintext);
    view.showDecoded(decodeEnvelope(plaintext));
    util.logTiming('consume_total', t0, performance.now());
    view.showAckResult(await api.acknowledge(claim));
  }

  function handlePreview(params) {
    view.showMessage(params.get('text') || PREVIEW_TEXT);
    view.setStatus('Decrypted (preview)');
  }

  // readFragment validates location.hash (one leading "#" removed) before
  // any network request. Returns {frag, problem}; problem is a status message.
  function readFragment(hash) {
    try {
      return { frag: window.goneCrypto.parseFragment(String(hash).replace(/^#/, '')), problem: '' };
    } catch (e) {
      return { frag: null, problem: FRAGMENT_PROBLEMS.get(e.code) || BAD_FRAGMENT };
    }
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
    const parsed = readFragment(location.hash);
    if (parsed.problem) {
      view.setStatus(parsed.problem);
      return;
    }
    if (!registerFromPath()) {
      view.setStatus('Invalid secret id');
      return;
    }
    window.addEventListener('beforeunload', view.guardUnload);
    window.addEventListener('pagehide', view.cleanup);
    run(parsed.frag).catch(fail);
  }

  start();
})();
