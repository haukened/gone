'use strict';

// DOM side of the single-request page, /request/{id}. Views: #view-check
// (loading and load errors), #view-missing (not saved in this browser),
// #view-waiting (facts, Check again, Cancel with an inline confirm),
// #view-cancelled, and the receive page's #view-open, #view-revealed and
// #view-gone, which consumeView drives once the reply is opened. Text is set
// as goneI18n message keys; the requester's own label is shown as typed.
// Exposed as window.goneRequestDetailView.
(function requestDetailViewModule() {
  if (window.goneRequestDetailView || !window.goneI18n) return;
  const i18n = window.goneI18n;

  const VIEWS = ['check', 'missing', 'waiting', 'cancelled', 'open', 'revealed', 'gone'];
  const TITLES = {
    missing: 'js.detail.titleMissing',
    waiting: 'js.detail.titleWaiting',
    cancelled: 'js.detail.titleCancelled',
    open: 'js.detail.titleOpen',
    gone: 'js.detail.titleGone'
  };

  function byId(id) {
    return document.getElementById(id);
  }

  function show(node, visible) {
    if (node) node.hidden = !visible;
  }

  // switchTo shows the named view, hides the others, and moves focus to its
  // heading so screen readers announce the change.
  function switchTo(name) {
    VIEWS.forEach(function (v) { show(byId(`view-${v}`), v === name); });
    if (TITLES[name]) i18n.setTitle(TITLES[name]);
    const heading = byId(`${name}-heading`);
    if (heading) heading.focus();
  }

  // setTime writes a time in the page's language (style "datetime" unless
  // given) and the machine-readable datetime.
  function setTime(id, when, style) {
    const node = byId(id);
    if (!node) return;
    i18n.value(node, { date: when.toISOString(), style: style || 'datetime' });
    node.setAttribute('datetime', when.toISOString());
  }

  // showCheckError shows a load error message key.
  function showCheckError(key, retryable) {
    i18n.clear(byId('check-status'));
    i18n.set(byId('check-error-text'), key);
    show(byId('check-error'), true);
    show(byId('check-retry'), retryable);
  }

  // showEntry fills what this browser knows about the request.
  function showEntry(entry) {
    ['waiting-label', 'open-label'].forEach(function (id) {
      if (entry.label) i18n.plain(byId(id), entry.label);
    });
    const link = byId('waiting-link');
    if (link) link.value = entry.replyLink;
  }

  // showWaiting renders a waiting status; a repeat check only updates the
  // facts and announces the time.
  function showWaiting(status, checkedAt, repeat) {
    setTime('request-created', status.createdAt);
    setTime('request-expires', status.expiresAt);
    setTime('request-checked', checkedAt, 'time');
    if (repeat) i18n.set(byId('waiting-status'), 'js.detail.stillWaiting', { time: { date: checkedAt.toISOString(), style: 'time' } });
    else switchTo('waiting');
  }

  function showReady(status) {
    setTime('open-expires', status.expiresAt);
    switchTo('open');
  }

  function setBusyButton(id, busy, busyKey, idleKey) {
    const btn = byId(id);
    if (!btn) return;
    btn.setAttribute('aria-disabled', String(busy));
    i18n.set(btn.querySelector('span') || btn, busy ? busyKey : idleKey);
  }

  // showWaitingError shows an error message key, or hides it when key is ''.
  function showWaitingError(key) {
    if (key) i18n.set(byId('waiting-error-text'), key);
    else i18n.clear(byId('waiting-error-text'));
    show(byId('waiting-error'), Boolean(key));
  }

  function openConfirm() {
    show(byId('cancel-start'), false);
    show(byId('cancel-confirm'), true);
    const text = byId('cancel-confirm-text');
    if (text) text.focus();
  }

  function closeConfirm() {
    show(byId('cancel-confirm'), false);
    show(byId('cancel-start'), true);
    const btn = byId('cancel-now');
    if (btn) btn.focus();
  }

  function on(id, handler) {
    const node = byId(id);
    if (node) node.addEventListener('click', handler);
  }

  // bind wires the waiting view's buttons to controller handlers.
  function bind(c) {
    on('check-retry', c.check);
    on('check-again', c.check);
    on('cancel-now', c.ask);
    on('cancel-yes', c.confirm);
    on('cancel-no', c.keep);
    on('copy-waiting-link', c.copy);
    const confirm = byId('cancel-confirm');
    if (confirm) confirm.addEventListener('keydown', function (ev) { if (ev.key === 'Escape') c.keep(); });
  }

  window.goneRequestDetailView = Object.freeze({
    byId: byId,
    switchTo: switchTo,
    showCheckError: showCheckError,
    showEntry: showEntry,
    showWaiting: showWaiting,
    showReady: showReady,
    showWaitingError: showWaitingError,
    setChecking: function (busy) { setBusyButton('check-again', busy, 'js.common.checking', 'js.manage.checkAgain'); },
    setCancelling: function (busy) { setBusyButton('cancel-yes', busy, 'js.detail.cancelling', 'js.detail.cancelIt'); },
    openConfirm: openConfirm,
    closeConfirm: closeConfirm,
    bind: bind
  });
})();
