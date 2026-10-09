'use strict';

// The Open button flow shared by the secret page and the request page: claim
// (streamed GET) -> decrypt -> decode -> ack (DELETE). The ciphertext is only
// deleted from the server once the full payload has been received and
// authenticated, so interrupted downloads can retry. A download whose
// decryption may be retried (a v2 passphrase) is kept in memory so the retry
// needs no further request. The caller supplies how to decrypt. Exposed as
// window.goneConsumeOpener.
(function consumeOpenerModule() {
  if (window.goneConsumeOpener) return;
  const util = window.goneUtil;
  const api = window.goneConsumeApi;
  const view = window.goneConsumeView;
  if (!util || !util.allPresent([window.goneEnvelope, api, view])) return;

  const MALFORMED = 'js.consume.damaged';
  const UNEXPECTED = 'js.consume.unexpected';
  const ERASED = 'js.consume.erased';

  function decodeEnvelope(plaintext) {
    try {
      return window.goneEnvelope.decode(plaintext);
    } catch (e) {
      console.error('[gone] invalid envelope', e);
      throw api.FetchError(MALFORMED, false);
    }
  }

  // run claims, decrypts and renders, then acknowledges deletion. claim
  // persists across attempts so a retry presents the same claim token;
  // held.fetched keeps a download whose passphrase was wrong, so the retry
  // only decrypts again.
  async function run(source, claim, held) {
    const t0 = performance.now();
    if (!held.fetched) {
      view.setStatus('js.consume.retrieving');
      held.fetched = await api.fetchWithRetry(claim, view.setProgress);
      util.logTiming('consume_fetch', t0, performance.now());
    }
    const plaintext = await source.decrypt(held.fetched).catch(function (e) {
      if (!e.passphrase) held.fetched = null;
      throw e;
    });
    held.fetched = null;
    view.keepPlaintext(plaintext);
    view.showDecoded(decodeEnvelope(plaintext));
    util.logTiming('consume_total', t0, performance.now());
    const acked = await api.acknowledge(claim);
    view.showAckResult(acked);
    if (source.onOpened) source.onOpened(acked);
  }

  // create returns the Open button handler for source:
  //   decrypt(fetched): Promise of the plaintext for a {resp, body} download;
  //     a rejection with .passphrase keeps the download for a retry.
  //   wipe(): zeroes the key material when the page is left.
  //   onGone(), onOpened(acked): optional notifications.
  // Retryable failures leave the button usable; anything else locks it.
  function create(source) {
    const claim = { token: '' };
    const held = { fetched: null };
    const flags = { busy: false, done: false, guarded: false };

    // onPageHide clears the page and zeroes a download kept for a
    // passphrase retry, locking Open since the secret can't be fetched again.
    function onPageHide() {
      view.cleanup();
      if (!held.fetched) return;
      held.fetched.body.fill(0);
      source.wipe();
      held.fetched = null;
      flags.done = true;
      view.showError(ERASED);
      view.disableOpen();
    }

    function settleGone() {
      view.showGone();
      if (source.onGone) source.onGone();
    }

    function fail(e) {
      if (flags.done) { // wiped by pagehide mid-attempt
        view.setOpening(false);
        return;
      }
      const known = api.isFetchError(e);
      flags.busy = false;
      flags.done = !(known && e.retryable);
      view.setOpening(false);
      if (api.isGone(e)) {
        settleGone();
        return;
      }
      console.error('[gone] consume error', known ? e.message : e);
      view.showError(known ? e.message : UNEXPECTED);
      if (e.passphrase) view.passphraseFailed();
      if (flags.done) view.disableOpen();
    }

    return function open() {
      if (flags.busy || flags.done) return;
      if (view.passphraseMissing()) {
        view.focusPassphrase();
        return;
      }
      flags.busy = true;
      view.clearError();
      view.setOpening(true);
      if (!flags.guarded) {
        flags.guarded = true;
        window.addEventListener('beforeunload', view.guardUnload);
        window.addEventListener('pagehide', onPageHide);
      }
      run(source, claim, held).then(function () { flags.done = true; }, fail);
    };
  }

  window.goneConsumeOpener = Object.freeze({ create: create });
})();
