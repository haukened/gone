'use strict';

// Secret consumption flow: claim (streamed GET) -> decrypt -> decode -> ack (DELETE).
// Nothing is fetched until the recipient presses Open, so link-preview bots
// can't burn the secret. The secret is only deleted from the server once the
// full payload has been received and authenticated by AES-GCM, so interrupted
// downloads can retry. Network/crypto live in consumeApi.js; DOM rendering in
// consumeView.js.
(function consumeFlow() {
  const util = window.goneUtil;
  const api = window.goneConsumeApi;
  const view = window.goneConsumeView;
  if (!util || !util.allPresent([window.goneCrypto, window.goneEnvelope, api, view]) || !view.present) return;

  const MALFORMED = 'This secret\u2019s contents are damaged. Ask the sender to share it again.';
  const BAD_FRAGMENT = 'This link is missing its key, so the secret can\u2019t be decrypted. Check that you copied the whole link, including everything after the #.';
  const BAD_ID = 'This link isn\u2019t valid. Check that you copied all of it.';
  const UNEXPECTED = 'Something went wrong opening this secret. Try again.';
  const FRAGMENT_PROBLEMS = new Map([['unsupported_version', 'This link was made by a newer version of Gone and can\u2019t be opened here.']]);
  const PREVIEW_TEXT = 'This is a preview of a decrypted secret. Customize via ?text=...';

  function decodeEnvelope(plaintext) {
    try {
      return window.goneEnvelope.decode(plaintext);
    } catch (e) {
      console.error('[gone] invalid envelope', e);
      throw api.FetchError(MALFORMED, false);
    }
  }

  // run claims, decrypts and renders the secret, then acknowledges deletion.
  // claim persists across attempts so a retry presents the same claim token.
  async function run(frag, claim) {
    const t0 = performance.now();
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

  // readFragment validates location.hash (one leading "#" removed) before
  // any network request. Returns {frag, problem}; problem is a user message.
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
    } catch {
      return false;
    }
  }

  // validate checks the link before anything touches the network.
  // Returns {frag, problem}.
  function validate() {
    const parsed = readFragment(location.hash);
    if (parsed.problem) return parsed;
    if (!registerFromPath()) return { frag: null, problem: BAD_ID };
    return parsed;
  }

  // makeOpener returns the Open button handler. Retryable failures leave the
  // button usable; anything else locks it.
  function makeOpener(frag) {
    const claim = { token: '' };
    const flags = { busy: false, done: false, guarded: false };

    function fail(e) {
      const known = api.isFetchError(e);
      flags.busy = false;
      flags.done = !(known && e.retryable);
      view.setOpening(false);
      if (api.isGone(e)) {
        view.showGone();
        return;
      }
      console.error('[gone] consume error', known ? e.message : e);
      view.showError(known ? e.message : UNEXPECTED);
      if (flags.done) view.disableOpen();
    }

    return function open() {
      if (flags.busy || flags.done) return;
      flags.busy = true;
      view.clearError();
      view.setOpening(true);
      if (!flags.guarded) {
        flags.guarded = true;
        window.addEventListener('beforeunload', view.guardUnload);
        window.addEventListener('pagehide', view.cleanup);
      }
      run(frag, claim).then(function () { flags.done = true; }, fail);
    };
  }

  function start() {
    const params = new URLSearchParams(location.search);
    if (params.get('preview') === 'secret') {
      view.showDecoded({ message: params.get('text') || PREVIEW_TEXT, files: [] });
      return;
    }
    const checked = validate();
    if (checked.problem) {
      view.showError(checked.problem);
      view.disableOpen();
      return;
    }
    view.onOpen(makeOpener(checked.frag));
  }

  start();
})();
