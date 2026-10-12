'use strict';

// Actions on the single-request page, /request/{id}: forgetting the local
// entry, cancelling the request, copying its link, and turning errors into
// message keys. Exposed as window.goneRequestDetailActions.
(function requestDetailActionsModule() {
  const util = window.goneUtil;
  const view = window.goneRequestDetailView;
  if (window.goneRequestDetailActions || !util || !util.allPresent([view, window.goneRequestStore, window.goneRequestApi])) return;
  const store = window.goneRequestStore;
  const api = window.goneRequestApi;

  const UNEXPECTED = 'js.common.unexpected';

  function message(e) {
    if (api.isRequestError(e)) return e.message;
    console.error('[gone] request error', e);
    return UNEXPECTED;
  }

  function forget(id) {
    return store.remove(id).catch(function () {});
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
    if (ok) util.flashCopied(view.byId('copy-waiting-link'), status, 'js.request.linkCopied');
  }

  window.goneRequestDetailActions = Object.freeze({ message: message, forget: forget, cancelRequest: cancelRequest, copyLink: copyLink });
})();
