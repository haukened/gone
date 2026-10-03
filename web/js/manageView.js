'use strict';

// DOM side of the sender's manage page. The page has four server-rendered
// views: #view-check (first load, link problems and load errors),
// #view-pending (status facts, Check again, Delete with an inline confirm),
// #view-deleted and #view-gone. Requires window.goneUtil. Exposed as
// window.goneManageView.
(function manageViewModule() {
  if (window.goneManageView || !window.goneUtil) return;
  const util = window.goneUtil;

  const VIEWS = ['check', 'pending', 'deleted', 'gone'];
  const TITLES = {
    pending: 'Gone \u00b7 Your secret is waiting',
    deleted: 'Gone \u00b7 Secret deleted',
    gone: 'Gone \u00b7 This secret is gone'
  };
  const DATE_FMT = { dateStyle: 'medium', timeStyle: 'short' };
  const TIME_FMT = { timeStyle: 'short' };

  function byId(id) {
    return document.getElementById(id);
  }

  // lookupDom finds the manage page elements; any may be null.
  function lookupDom() {
    const ids = {
      checkHeading: 'check-heading', checkStatus: 'check-status', checkError: 'check-error', checkErrorText: 'check-error-text',
      retry: 'check-retry', created: 'manage-created', expires: 'manage-expires',
      checked: 'manage-checked', again: 'check-again', pendingStatus: 'pending-status',
      pendingError: 'pending-error', pendingErrorText: 'pending-error-text',
      deleteStart: 'delete-start', deleteNow: 'delete-now', confirm: 'delete-confirm',
      confirmText: 'delete-confirm-text', yes: 'delete-yes', no: 'delete-no'
    };
    const dom = {};
    Object.keys(ids).forEach(function (k) { dom[k] = byId(ids[k]); });
    return dom;
  }

  const dom = lookupDom();
  const state = { current: 'check' };

  function show(node, visible) {
    if (node) node.hidden = !visible;
  }

  function setDisabled(node, disabled) {
    if (node) node.disabled = disabled;
  }

  // setLabel replaces a button's <span> label, or its text when it has none.
  function setLabel(btn, text) {
    if (!btn) return;
    const span = btn.querySelector('span');
    util.setText(span || btn, text);
  }

  // switchTo shows the named view, hides the others, and moves focus to its
  // heading so screen readers announce the change.
  function switchTo(name) {
    VIEWS.forEach(function (v) { show(byId(`view-${v}`), v === name); });
    if (TITLES[name]) document.title = TITLES[name];
    state.current = name;
    const heading = byId(`${name}-heading`);
    if (heading) heading.focus();
  }

  // setTime writes a human time and the machine-readable datetime.
  function setTime(node, when, fmt) {
    if (!node) return;
    node.textContent = when.toLocaleString(undefined, fmt);
    node.setAttribute('datetime', when.toISOString());
  }

  // showChecking resets the first-load view to its busy state.
  function showChecking() {
    util.setText(dom.checkHeading, 'Checking on your secret.');
    util.setText(dom.checkStatus, 'Checking\u2026');
    show(dom.checkError, false);
    show(dom.retry, false);
  }

  // showCheckError reports a first-load problem. retryable shows Try again.
  function showCheckError(msg, retryable) {
    util.setText(dom.checkHeading, 'Couldn\u2019t check your secret.');
    util.setText(dom.checkStatus, '');
    util.setText(dom.checkErrorText, msg);
    show(dom.checkError, true);
    show(dom.retry, retryable);
  }

  // showPending fills the status facts and shows the pending view. A repeat
  // check keeps focus on Check again and announces the result instead.
  function showPending(info, now) {
    setTime(dom.created, info.createdAt, DATE_FMT);
    setTime(dom.expires, info.expiresAt, DATE_FMT);
    setTime(dom.checked, now, TIME_FMT);
    if (state.current === 'pending') {
      util.setText(dom.pendingStatus, `Still waiting as of ${dom.checked ? dom.checked.textContent : 'now'}. Nobody has opened it.`);
      return;
    }
    util.setText(dom.pendingStatus, '');
    switchTo('pending');
  }

  // setChecking shows Check again as busy, and blocks Delete meanwhile.
  function setChecking(busy) {
    setDisabled(dom.again, busy);
    setDisabled(dom.deleteNow, busy);
    setLabel(dom.again, busy ? 'Checking\u2026' : 'Check again');
    if (busy) util.setText(dom.pendingStatus, 'Checking\u2026');
  }

  // setDeleting shows the confirm buttons as busy.
  function setDeleting(busy) {
    setDisabled(dom.yes, busy);
    setDisabled(dom.no, busy);
    setDisabled(dom.again, busy);
    setLabel(dom.yes, busy ? 'Deleting\u2026' : 'Delete it');
  }

  function showPendingError(msg) {
    util.setText(dom.pendingStatus, '');
    util.setText(dom.pendingErrorText, msg);
    show(dom.pendingError, true);
  }

  function clearPendingError() {
    show(dom.pendingError, false);
  }

  // openConfirm swaps Delete now for the inline confirm and focuses its prompt.
  function openConfirm() {
    show(dom.deleteStart, false);
    show(dom.confirm, true);
    if (dom.confirmText) dom.confirmText.focus();
  }

  // closeConfirm hides the confirm and returns focus to Delete now.
  function closeConfirm() {
    show(dom.confirm, false);
    show(dom.deleteStart, true);
    if (dom.deleteNow) dom.deleteNow.focus();
  }

  function on(node, type, fn) {
    if (node) node.addEventListener(type, fn);
  }

  // bind wires page controls to handlers {retry, check, ask, confirm, cancel}.
  // Escape inside the confirm cancels it.
  function bind(handlers) {
    on(dom.retry, 'click', handlers.retry);
    on(dom.again, 'click', handlers.check);
    on(dom.deleteNow, 'click', handlers.ask);
    on(dom.yes, 'click', handlers.confirm);
    on(dom.no, 'click', handlers.cancel);
    on(dom.confirm, 'keydown', function (ev) {
      if (ev.key === 'Escape') handlers.cancel();
    });
  }

  window.goneManageView = Object.freeze({
    present: Boolean(byId('view-check') && byId('view-pending')),
    showChecking: showChecking,
    showCheckError: showCheckError,
    showPending: showPending,
    setChecking: setChecking,
    setDeleting: setDeleting,
    showPendingError: showPendingError,
    clearPendingError: clearPendingError,
    openConfirm: openConfirm,
    closeConfirm: closeConfirm,
    showDeleted: function () { switchTo('deleted'); },
    showGone: function () { switchTo('gone'); },
    bind: bind
  });
})();
