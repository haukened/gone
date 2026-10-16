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

  // ready is set once a valid link has been set up. Until then a hashchange
  // (the full link pasted into this tab, a same-document navigation that
  // runs no script again) may set the page up; afterwards it never does, so
  // handlers are bound once and a check in flight is never replaced.
  let ready = false;

  function setUp() {
    const token = readLink();
    // The token now lives only in memory; a reload shows the invalid-link page.
    util.clearFragment();
    if (!token) {
      view.showCheckError(api.INVALID_LINK, false);
      return;
    }
    ready = true;
    const controller = makeController(token);
    view.bind(controller);
    controller.check();
  }

  function onHashChange() {
    if (!ready && location.hash) setUp();
  }

  function start() {
    // Development builds only: web/dev/js/preview.js defines the hook.
    if (window.gonePreview && window.gonePreview.manage(view, api)) return;
    window.addEventListener('hashchange', onHashChange);
    setUp();
  }

  start();
})();
