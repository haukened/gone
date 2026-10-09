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

  const BAD_FRAGMENT = 'This link is missing its key, so the secret can\u2019t be decrypted. Check that you copied the whole link, including everything after the #.';
  const NO_CRYPTO = 'This browser only decrypts on secure (HTTPS) pages, so this secret can\u2019t be opened here. Nothing was downloaded, and the link still works. Ask the sender for an HTTPS link.';
  const BAD_ID = 'This link isn\u2019t valid. Check that you copied all of it.';
  const FRAGMENT_PROBLEMS = new Map([['unsupported_version', 'This link was made by a newer version of Gone and can\u2019t be opened here.']]);
  const PREVIEW_TEXT = 'This is a preview of a decrypted secret. Customize via ?text=...';

  // source tells the shared opener how to decrypt this link: v1 with the
  // link key alone, v2 with the link key and the typed passphrase.
  function source(frag) {
    return {
      decrypt: function (fetched) {
        if (frag.version === window.goneCrypto.versionV2) {
          view.setStatus('Unlocking\u2026');
          return api.decryptV2(fetched.resp, fetched.body, frag, view.passphrase());
        }
        view.setStatus('Decrypting\u2026');
        return api.decrypt(fetched.resp, fetched.body, frag);
      },
      wipe: function () { frag.key.fill(0); }
    };
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

  // validate checks the link and browser before anything touches the network.
  // Returns {frag, problem}.
  function validate() {
    if (!util.cryptoAvailable()) return { frag: null, problem: NO_CRYPTO };
    const parsed = readFragment(location.hash);
    if (parsed.problem) return parsed;
    if (!registerFromPath()) return { frag: null, problem: BAD_ID };
    return parsed;
  }

  function start() {
    const params = new URLSearchParams(location.search);
    if (params.get('preview') === 'secret') {
      view.showDecoded({ message: params.get('text') || PREVIEW_TEXT, files: [] });
      return;
    }
    // Every link gets the same Open page, valid or not, so loading it reveals
    // nothing. A link that can't work says why only when Open is pressed,
    // still without touching the network.
    const checked = validate();
    // Missing WebCrypto says nothing about the link, so Open is locked at
    // once; the insecure-page banner explains why.
    if (!util.cryptoAvailable()) view.disableOpen();
    if (checked.problem) {
      view.onOpen(function () {
        view.showError(checked.problem);
        view.disableOpen();
      });
      return;
    }
    if (checked.frag.version === window.goneCrypto.versionV2) view.showPassphrase();
    view.onOpen(opener.create(source(checked.frag)));
  }

  start();
})();
