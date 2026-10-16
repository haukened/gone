'use strict';

// DOM side of the sender's manage page. The page has four server-rendered
// views: #view-check (first load, link problems and load errors),
// #view-pending (status facts, Check again, Delete with an inline confirm),
// #view-deleted and #view-gone. Text is set as goneI18n message keys.
// Exposed as window.goneManageView.
(function manageViewModule() {
  if (window.goneManageView || !window.goneI18n || !window.goneUtil || !window.goneInlineConfirm) return;
  const i18n = window.goneI18n;

  const VIEWS = ['check', 'pending', 'deleted', 'gone'];
  const TITLES = {
    pending: 'js.manage.titlePending',
    deleted: 'js.manage.titleDeleted',
    gone: 'js.consume.titleGone'
  };

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
      deleteNow: 'delete-now', yes: 'delete-yes', no: 'delete-no'
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
  function setLabel(btn, key) {
    if (!btn) return;
    i18n.set(btn.querySelector('span') || btn, key);
  }

  // switchTo shows the named view, hides the others, and moves focus to its
  // heading so screen readers announce the change.
  function switchTo(name) {
    state.current = name;
    window.goneUtil.showView(VIEWS, name, TITLES);
  }

  // setTime writes a time in the page's language (style "datetime" or
  // "time") and the machine-readable datetime.
  function setTime(node, when, style) {
    if (!node) return;
    i18n.value(node, { date: when.toISOString(), style: style });
    node.setAttribute('datetime', when.toISOString());
  }

  // showChecking resets the first-load view to its busy state.
  function showChecking() {
    i18n.set(dom.checkHeading, 'js.manage.checkTitle');
    i18n.set(dom.checkStatus, 'js.common.checking');
    show(dom.checkError, false);
    show(dom.retry, false);
  }

  // showCheckError reports a first-load problem (a message key). retryable
  // shows Try again.
  function showCheckError(key, retryable) {
    i18n.set(dom.checkHeading, 'js.manage.checkFailed');
    i18n.clear(dom.checkStatus);
    i18n.set(dom.checkErrorText, key);
    show(dom.checkError, true);
    show(dom.retry, retryable);
  }

  // showPending fills the status facts and shows the pending view. A repeat
  // check keeps focus on Check again and announces the result instead.
  function showPending(info, now) {
    setTime(dom.created, info.createdAt, 'datetime');
    setTime(dom.expires, info.expiresAt, 'datetime');
    setTime(dom.checked, now, 'time');
    if (state.current === 'pending') {
      i18n.set(dom.pendingStatus, 'js.manage.stillWaiting', { time: { date: now.toISOString(), style: 'time' } });
      return;
    }
    i18n.clear(dom.pendingStatus);
    switchTo('pending');
  }

  // setChecking shows Check again as busy, and blocks Delete meanwhile.
  function setChecking(busy) {
    setDisabled(dom.again, busy);
    setDisabled(dom.deleteNow, busy);
    setLabel(dom.again, busy ? 'js.common.checking' : 'js.manage.checkAgain');
    if (busy) i18n.set(dom.pendingStatus, 'js.common.checking');
  }

  // setDeleting shows the confirm buttons as busy.
  function setDeleting(busy) {
    setDisabled(dom.yes, busy);
    setDisabled(dom.no, busy);
    setDisabled(dom.again, busy);
    setLabel(dom.yes, busy ? 'js.manage.deleting' : 'js.manage.deleteIt');
  }

  // showPendingError shows an error message key on the pending view.
  function showPendingError(key) {
    i18n.clear(dom.pendingStatus);
    i18n.set(dom.pendingErrorText, key);
    show(dom.pendingError, true);
  }

  function clearPendingError() {
    show(dom.pendingError, false);
  }

  // confirmBox swaps Delete now for the inline confirm and focuses its
  // prompt; closing it returns focus to Delete now.
  const confirmBox = window.goneInlineConfirm.create({
    start: 'delete-start', panel: 'delete-confirm', text: 'delete-confirm-text', button: 'delete-now'
  });

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
    confirmBox.onEscape(handlers.cancel);
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
    openConfirm: confirmBox.open,
    closeConfirm: confirmBox.close,
    showDeleted: function () { switchTo('deleted'); },
    showGone: function () { switchTo('gone'); },
    bind: bind
  });
})();
