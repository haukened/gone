'use strict';

// Development-only preview modes for design review: ?preview=... renders a
// static state with no network request. This file lives in web/dev/, which
// `task minify` never copies and the production build never embeds, so a
// release has no code path that reads ?preview. The development server
// (build tag dev) loads it through {{ if dev }} in the page templates, and
// the page scripts call these hooks only when window.gonePreview exists.
(function previewModule() {
  if (window.gonePreview) return;

  const CONSUME_TEXT = 'This is a preview of a decrypted secret. Customize via ?text=...';
  const MOCK_ID = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
  const MOCK_KEY = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';

  function mode() {
    return new URLSearchParams(location.search).get('preview');
  }

  // consume renders ?preview=secret (with optional &text=) in the secret
  // page. Returns whether a preview was shown.
  function consume(view) {
    if (mode() !== 'secret') return false;
    const text = new URLSearchParams(location.search).get('text');
    view.showDecoded({ message: text || CONSUME_TEXT, files: [] });
    return true;
  }

  // manage renders ?preview=pending|deleted|gone|error in the manage page.
  // Returns whether a preview was shown.
  function manage(view, api) {
    const now = Date.now();
    const modes = new Map([
      ['pending', function () {
        view.showPending({ createdAt: new Date(now - 5 * 60000), expiresAt: new Date(now + 55 * 60000) }, new Date(now));
      }],
      ['deleted', view.showDeleted],
      ['gone', view.showGone],
      ['error', function () { view.showCheckError(api.INVALID_LINK, false); }]
    ]);
    const render = modes.get(mode());
    if (typeof render !== 'function') return false;
    render();
    return true;
  }

  // submit renders ?preview=result (with optional &passphrase) in the create
  // page: a mock result panel. ctx is window.goneSubmitDom.
  function submit(ctx) {
    if (mode() !== 'result' || !window.goneResultPanel) return;
    const v2 = new URLSearchParams(location.search).has('passphrase');
    const version = v2 ? window.goneCrypto.versionV2 : window.goneCrypto.version;
    const future = new Date(Date.now() + 30 * 60 * 1000).toISOString();
    window.goneResultPanel.show({
      shareURL: `${location.origin}/secret/${MOCK_ID}#v${version}:${MOCK_KEY}`,
      manageURL: ctx.uploader.buildManageURL(MOCK_ID, 'B'.repeat(43)),
      expiresAt: future,
      passphrase: v2,
      focus: false
    });
  }

  window.gonePreview = Object.freeze({ consume: consume, manage: manage, submit: submit });
})();
