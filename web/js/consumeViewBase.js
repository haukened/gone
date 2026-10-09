'use strict';

// Shared DOM lookup and state for consume view modules. Text goes through
// window.goneI18n.
(function consumeViewBaseModule() {
  if (window.goneConsumeViewBase || !window.goneUtil || !window.goneI18n || !window.goneIcons) return;

  function byId(id) {
    return document.getElementById(id);
  }

  function lookupDom() {
    const open = byId('open-secret');
    return {
      open: open,
      openLabel: open ? open.querySelector('span') : null,
      status: byId('consume-status'),
      progress: byId('download-progress'),
      errorBox: byId('consume-error'),
      errorText: byId('consume-error-text'),
      revealedHeading: byId('revealed-heading'),
      ackWarning: byId('ack-warning'),
      messagePanel: byId('message-panel'),
      output: byId('secret-output'),
      copy: byId('copy-secret'),
      show: byId('show-secret'),
      showLabel: byId('show-secret') ? byId('show-secret').querySelector('span') : null,
      cover: byId('secret-cover'),
      coverDots: byId('secret-cover-dots'),
      size: byId('secret-size'),
      alwaysShow: byId('always-show'),
      copyStatus: byId('copy-status'),
      fileSection: byId('file-section'),
      fileList: byId('file-output-list'),
      downloadAll: byId('download-all'),
      passField: byId('open-pass-field'),
      pass: byId('open-passphrase'),
      passToggle: byId('open-pass-toggle'),
      passWarn: byId('open-pass-warn'),
      openHint: byId('open-hint'),
      steps: {
        waiting: byId('step-waiting'),
        waitingNote: byId('step-waiting-note'),
        opened: byId('step-opened'),
        openedNote: byId('step-opened-note'),
        goneNote: byId('step-gone-note')
      }
    };
  }

  const dom = lookupDom();
  const state = { plaintext: null, files: [], urls: [], pending: 0, busy: false, locked: false };
  state.openLabel = window.goneI18n.snapshot(dom.openLabel);
  const ctx = { util: window.goneUtil, i18n: window.goneI18n, icons: window.goneIcons, byId: byId, dom: dom, state: state, pass: null };

  // setStatus shows a status message key, or clears the status when key is
  // empty.
  function setStatus(key, args) {
    if (!dom.status) return;
    if (key) ctx.i18n.set(dom.status, key, args);
    else ctx.i18n.clear(dom.status);
  }

  function syncOpen() {
    const missing = ctx.pass ? ctx.pass.missing() : false;
    if (dom.open) dom.open.setAttribute('aria-disabled', String(state.busy || state.locked || missing));
  }

  ctx.setStatus = setStatus;
  ctx.syncOpen = syncOpen;
  window.goneConsumeViewBase = ctx;
})();
