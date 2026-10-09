'use strict';

// The single-request page, /request/{id}. It finds the request in this
// browser's IndexedDB (the URL carries only the ID), checks its status, and
// polls while it waits. When the reply is ready, the shared opener claims
// it, this browser's non-extractable private key decrypts it, and the
// acknowledgement deletes it from the server; the local entry is then
// forgotten too. Nothing about the request's key ever leaves the browser.
(function requestDetailFlow() {
  const util = window.goneUtil;
  const view = window.goneRequestDetailView;
  const deps = [view, window.goneCryptoV3, window.goneCryptoEncoding, window.goneRequestStore, window.goneRequestApi,
    window.goneRequestPoll, window.goneConsumeApi, window.goneConsumeView, window.goneConsumeOpener];
  if (!util || !util.allPresent(deps) || !view.byId('view-waiting')) return;
  const store = window.goneRequestStore;
  const api = window.goneRequestApi;
  const consumeApi = window.goneConsumeApi;
  const consumeView = window.goneConsumeView;

  const UNEXPECTED = 'Something went wrong. Try again.';
  const NO_CRYPTO = 'This browser only decrypts on secure (HTTPS) pages, so the reply can\u2019t be opened here.';
  const VERIFY_ERROR = 'Couldn\u2019t decrypt this reply. It may have been damaged on the way. Ask them to send it again with a new request.';
  const NONCE_BYTES = 12;

  function message(e) {
    if (api.isRequestError(e)) return e.message;
    console.error('[gone] request error', e);
    return UNEXPECTED;
  }

  function forget(id) {
    return store.remove(id).catch(function () {});
  }

  function readNonce(resp) {
    if (resp.headers.get('X-Gone-Version') !== String(window.goneCryptoV3.version)) throw new Error('version');
    const nonce = window.goneCryptoEncoding.b64urlDecode(resp.headers.get('X-Gone-Nonce') || '');
    if (nonce.length !== NONCE_BYTES) throw new Error('nonce');
    return nonce;
  }

  // source tells the shared opener how to open this reply.
  function source(entry) {
    return {
      decrypt: async function (fetched) {
        consumeView.setStatus('Decrypting\u2026');
        try {
          return await window.goneCryptoV3.decryptV3(fetched.body, readNonce(fetched.resp), entry.privateKey, entry.publicKey);
        } catch {
          throw consumeApi.FetchError(VERIFY_ERROR, false);
        } finally {
          fetched.body.fill(0);
        }
      },
      wipe: function () {},
      onOpened: function () { forget(entry.id); },
      onGone: function () { forget(entry.id); }
    };
  }

  // makeController returns the page's handlers for one saved request.
  function makeController(entry) {
    const flags = { busy: false, loaded: false, opener: false };
    const poll = window.goneRequestPoll.create(check);

    function becomeReady(status) {
      view.showReady(status);
      if (flags.opener) return;
      flags.opener = true;
      consumeApi.registerReplyEndpoint(entry.id, entry.manageToken);
      consumeView.onOpen(window.goneConsumeOpener.create(source(entry)));
      if (!util.cryptoAvailable()) {
        consumeView.showError(NO_CRYPTO);
        consumeView.disableOpen();
      }
    }

    // settle renders a status and tells the poller whether to keep going.
    function settle(status) {
      if (status.state === 'gone') {
        forget(entry.id);
        view.switchTo('gone');
        return 'stop';
      }
      if (status.state === 'ready') {
        becomeReady(status);
        return 'stop';
      }
      view.showWaiting(status, new Date(), flags.loaded);
      flags.loaded = true;
      return 'continue';
    }

    async function check() {
      if (flags.busy) return 'continue';
      flags.busy = true;
      view.setChecking(true);
      view.showWaitingError('');
      try {
        return settle(await api.status(entry.id, entry.manageToken));
      } catch (e) {
        if (flags.loaded) view.showWaitingError(message(e));
        else view.showCheckError(message(e), !api.isRequestError(e) || e.retryable);
        throw e;
      } finally {
        flags.busy = false;
        view.setChecking(false);
      }
    }

    return {
      start: poll.start,
      check: function () { poll.now(); },
      ask: function () { if (!flags.busy) view.openConfirm(); },
      keep: function () { if (!flags.busy) view.closeConfirm(); },
      confirm: function () { return cancelRequest(entry, flags, poll); },
      copy: copyLink
    };
  }

  // cancelRequest deletes the request on the server and forgets it here.
  // flags.busy serialises it with status checks.
  async function cancelRequest(entry, flags, poll) {
    if (flags.busy) return;
    flags.busy = true;
    view.setCancelling(true);
    try {
      const outcome = await api.cancel(entry.id, entry.manageToken);
      poll.stop();
      await forget(entry.id);
      view.switchTo(outcome === 'cancelled' ? 'cancelled' : 'gone');
    } catch (e) {
      view.closeConfirm();
      view.showWaitingError(message(e));
    } finally {
      flags.busy = false;
      view.setCancelling(false);
    }
  }

  async function copyLink() {
    const input = view.byId('waiting-link');
    const status = view.byId('waiting-copy-status');
    const ok = await util.copyText(input.value, function () { input.focus(); input.select(); }, status);
    if (ok) util.flashCopied(view.byId('copy-waiting-link'), status, 'Request link copied to clipboard.');
  }

  async function load(id) {
    if (!api.isID(id)) return null;
    try {
      return (await store.get(id)) || null;
    } catch {
      return null;
    }
  }

  async function start() {
    const parts = location.pathname.split('/');
    const entry = await load(parts[parts.length - 1]);
    if (!entry || entry.expiresAt <= Date.now()) {
      if (entry) forget(entry.id);
      view.switchTo('missing');
      return;
    }
    view.showEntry(entry);
    const controller = makeController(entry);
    view.bind(controller);
    controller.start();
  }

  start();
})();
