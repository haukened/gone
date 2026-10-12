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
  const actions = window.goneRequestDetailActions;
  const deps = [view, actions, window.goneRequestDetailSource, window.goneRequestPoll, window.goneConsumeApi,
    window.goneConsumeView, window.goneConsumeOpener];
  if (!util || !util.allPresent(deps) || !view.byId('view-waiting')) return;
  const store = window.goneRequestStore;
  const api = window.goneRequestApi;
  const consumeApi = window.goneConsumeApi;
  const consumeView = window.goneConsumeView;
  const source = window.goneRequestDetailSource.source;
  const message = actions.message;
  const forget = actions.forget;

  const NO_CRYPTO = 'js.detail.noCrypto';

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
      confirm: function () { return actions.cancelRequest(entry, flags, poll); },
      copy: actions.copyLink
    };
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
