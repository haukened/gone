'use strict';

// Sender's manage page flow. The link is /manage/{id}#{token}; both parts
// are validated before any request, and the token only ever travels in the
// X-Gone-Manage header. The page checks status once on load, then on
// "Check again"; deleting asks for confirmation inline first. Network lives
// in manageApi.js; DOM in manageView.js.
(function manageFlow() {
  const util = window.goneUtil;
  const api = window.goneManageApi;
  const view = window.goneManageView;
  if (!util || !util.allPresent([api, view]) || !view.present) return;

  const UNEXPECTED = 'js.common.unexpected';

  // message returns the message key for an error.
  function message(e) {
    if (api.isManageError(e)) return e.message;
    console.error('[gone] manage error', e);
    return UNEXPECTED;
  }

  // readLink validates the id in the path and the token in the fragment.
  // Returns the token, or '' when the link is malformed.
  function readLink() {
    const token = String(location.hash).replace(/^#/, '');
    if (!api.isToken(token)) return '';
    const parts = location.pathname.split('/');
    try {
      api.registerEndpoint(parts[parts.length - 1]);
    } catch {
      return '';
    }
    return token;
  }

  // makeController returns the page's handlers for one validated token.
  // busy serialises status and revoke so they never overlap.
  function makeController(token) {
    const flags = { busy: false, loaded: false };

    function settle(result) {
      if (result.state === 'gone') {
        view.showGone();
        return;
      }
      flags.loaded = true;
      view.showPending(result, new Date());
    }

    function failCheck(e) {
      if (flags.loaded) view.showPendingError(message(e));
      else view.showCheckError(message(e), !api.isManageError(e) || e.retryable);
    }

    async function check() {
      if (flags.busy) return;
      flags.busy = true;
      if (flags.loaded) {
        view.clearPendingError();
        view.setChecking(true);
      } else {
        view.showChecking();
      }
      try {
        settle(await api.status(token));
      } catch (e) {
        failCheck(e);
      } finally {
        flags.busy = false;
        view.setChecking(false);
      }
    }

    async function confirm() {
      if (flags.busy) return;
      flags.busy = true;
      view.clearPendingError();
      view.setDeleting(true);
      try {
        const outcome = await api.revoke(token);
        if (outcome === 'deleted') view.showDeleted();
        else view.showGone();
      } catch (e) {
        view.closeConfirm();
        view.showPendingError(message(e));
      } finally {
        flags.busy = false;
        view.setDeleting(false);
      }
    }

    function ask() {
      if (!flags.busy) view.openConfirm();
    }

    function cancel() {
      if (!flags.busy) view.closeConfirm();
    }

    return { retry: check, check: check, ask: ask, confirm: confirm, cancel: cancel };
  }

  // preview renders a static state for design review (?preview=...) without
  // any network request. Returns whether a preview was shown.
  function preview(mode) {
    const now = Date.now();
    const modes = new Map([
      ['pending', function () {
        view.showPending({ createdAt: new Date(now - 5 * 60000), expiresAt: new Date(now + 55 * 60000) }, new Date(now));
      }],
      ['deleted', view.showDeleted],
      ['gone', view.showGone],
      ['error', function () { view.showCheckError(api.INVALID_LINK, false); }]
    ]);
    const render = modes.get(mode);
    if (typeof render !== 'function') return false;
    render();
    return true;
  }

  function start() {
    if (preview(new URLSearchParams(location.search).get('preview'))) return;
    const token = readLink();
    if (!token) {
      view.showCheckError(api.INVALID_LINK, false);
      return;
    }
    const controller = makeController(token);
    view.bind(controller);
    controller.check();
  }

  start();
})();
