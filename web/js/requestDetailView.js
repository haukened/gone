'use strict';

// DOM side of the single-request page, /request/{id}. Views: #view-check
// (loading and load errors), #view-missing (not saved in this browser),
// #view-waiting (facts, Check again, Cancel with an inline confirm),
// #view-cancelled, and the receive page's #view-open, #view-revealed and
// #view-gone, which consumeView drives once the reply is opened. Requires
// window.goneUtil. Exposed as window.goneRequestDetailView.
(function requestDetailViewModule() {
  if (window.goneRequestDetailView || !window.goneUtil) return;
  const util = window.goneUtil;

  const VIEWS = ['check', 'missing', 'waiting', 'cancelled', 'open', 'revealed', 'gone'];
  const TITLES = {
    missing: 'Gone \u00b7 Request not in this browser',
    waiting: 'Gone \u00b7 Waiting for a reply',
    cancelled: 'Gone \u00b7 Request cancelled',
    open: 'Gone \u00b7 Your reply is here',
    gone: 'Gone \u00b7 This request is gone'
  };
  const DATE_FMT = { dateStyle: 'medium', timeStyle: 'short' };
  const TIME_FMT = { timeStyle: 'short' };

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
    if (TITLES[name]) document.title = TITLES[name];
    const heading = byId(`${name}-heading`);
    if (heading) heading.focus();
  }

  // setTime writes a human time and the machine-readable datetime.
  function setTime(id, when, fmt) {
    const node = byId(id);
    if (!node) return;
    node.textContent = when.toLocaleString(undefined, fmt || DATE_FMT);
    node.setAttribute('datetime', when.toISOString());
  }

  function showCheckError(message, retryable) {
    util.setText(byId('check-status'), '');
    util.setText(byId('check-error-text'), message);
    show(byId('check-error'), true);
    show(byId('check-retry'), retryable);
  }

  // showEntry fills what this browser knows about the request.
  function showEntry(entry) {
    const label = entry.label || 'Your request';
    util.setText(byId('waiting-label'), label);
    util.setText(byId('open-label'), label);
    const link = byId('waiting-link');
    if (link) link.value = entry.replyLink;
  }

  // showWaiting renders a waiting status; a repeat check only updates the
  // facts and announces the time.
  function showWaiting(status, checkedAt, repeat) {
    setTime('request-created', status.createdAt);
    setTime('request-expires', status.expiresAt);
    setTime('request-checked', checkedAt, TIME_FMT);
    if (repeat) util.setText(byId('waiting-status'), `Still waiting as of ${checkedAt.toLocaleTimeString(undefined, TIME_FMT)}.`);
    else switchTo('waiting');
  }

  function showReady(status) {
    setTime('open-expires', status.expiresAt);
    switchTo('open');
  }

  function setBusyButton(id, busy, busyText, idleText) {
    const btn = byId(id);
    if (!btn) return;
    btn.setAttribute('aria-disabled', String(busy));
    util.setText(btn.querySelector('span') || btn, busy ? busyText : idleText);
  }

  function showWaitingError(message) {
    util.setText(byId('waiting-error-text'), message);
    show(byId('waiting-error'), Boolean(message));
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
    setChecking: function (busy) { setBusyButton('check-again', busy, 'Checking\u2026', 'Check again'); },
    setCancelling: function (busy) { setBusyButton('cancel-yes', busy, 'Cancelling\u2026', 'Cancel it'); },
    openConfirm: openConfirm,
    closeConfirm: closeConfirm,
    bind: bind
  });
})();
