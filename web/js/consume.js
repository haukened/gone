'use strict';

// Secret consumption page: validates the link, then hands the Open button to
// consumeOpener.js (claim -> decrypt -> decode -> ack). Nothing is fetched
// until the recipient presses Open, so link-preview bots can't burn the
// secret. A v2 (passphrase) link downloads once and keeps the ciphertext in
// memory, so a mistyped passphrase can be retried locally with no further
// network request. Network/crypto live in consumeApi.js; DOM rendering in
// consumeView.js.
(function consumeFlow() {
  const util = window.goneUtil;
  const api = window.goneConsumeApi;
  const view = window.goneConsumeView;
  const opener = window.goneConsumeOpener;
  if (!util || !util.allPresent([window.goneCrypto, api, view, opener]) || !view.present) return;

  const BAD_FRAGMENT = 'js.consume.badFragment';
  const NO_CRYPTO = 'js.consume.noCrypto';
  const BAD_ID = 'js.consume.invalidLink';
  const FRAGMENT_PROBLEMS = new Map([['unsupported_version', 'js.consume.newerVersion']]);

  // source tells the shared opener how to decrypt this link: v1 with the
  // link key alone, v2 with the link key and the typed passphrase.
  function source(frag) {
    return {
      decrypt: function (fetched) {
        if (frag.version === window.goneCrypto.versionV2) {
          view.setStatus('js.consume.unlocking');
          return api.decryptV2(fetched.resp, fetched.body, frag, view.passphrase());
        }
        view.setStatus('js.consume.decrypting');
        return api.decrypt(fetched.resp, fetched.body, frag);
      },
      wipe: function () { frag.key.fill(0); }
    };
  }

  // readFragment validates location.hash (one leading "#" removed) before
  // any network request. Returns {frag, problem}; problem is a message key.
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

  // validate checks the link and browser before anything touches the network.
  // Returns {frag, problem}.
  function validate() {
    if (!util.cryptoAvailable()) return { frag: null, problem: NO_CRYPTO };
    const parsed = readFragment(location.hash);
    if (parsed.problem) return parsed;
    if (!registerFromPath()) return { frag: null, problem: BAD_ID };
    return parsed;
  }

  // page is what the one Open click handler runs. problem is set while the
  // link can't work: only then may a later hashchange (the full link pasted
  // into this tab) set the page up again. A page set up for a valid link is
  // never touched again, so a claim in flight is never replaced.
  const page = { handler: null, problem: '' };

  // reportProblem makes Open explain the problem, without any request.
  function reportProblem(problem) {
    page.handler = function () {
      view.showError(problem);
      view.disableOpen();
    };
  }

  // configure reads and validates the link, then clears the fragment from
  // the address bar and points Open at the right handler.
  function configure() {
    const checked = validate();
    // The key now lives only in memory; a reload shows the incomplete-link page.
    util.clearFragment();
    // Missing WebCrypto says nothing about the link, so Open is locked at
    // once; the insecure-page banner explains why.
    if (!util.cryptoAvailable()) view.disableOpen();
    page.problem = checked.problem;
    if (checked.problem) {
      reportProblem(checked.problem);
      return;
    }
    if (checked.frag.version === window.goneCrypto.versionV2) view.showPassphrase();
    page.handler = opener.create(source(checked.frag));
  }

  // onHashChange recovers a page that was showing a link problem when the
  // person pastes the full link into this tab. That is a same-document
  // navigation, so no script would otherwise run again.
  function onHashChange() {
    if (!page.problem || !location.hash || !util.cryptoAvailable()) return;
    view.clearError();
    view.enableOpen();
    configure();
  }

  function start() {
    // Development builds only: web/dev/js/preview.js defines the hook.
    if (window.gonePreview && window.gonePreview.consume(view)) return;
    // Every link gets the same Open page, valid or not, so loading it reveals
    // nothing. A link that can't work says why only when Open is pressed,
    // still without touching the network.
    view.onOpen(function () { page.handler(); });
    window.addEventListener('hashchange', onHashChange);
    configure();
  }

  start();
})();
