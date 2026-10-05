'use strict';

// Shared DOM lookup and state for consume view modules.
(function consumeViewBaseModule() {
  if (window.goneConsumeViewBase || !window.goneUtil || !window.goneFileMeta || !window.goneIcons) return;

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
      hide: byId('hide-secret'),
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
      passWarn: byId('open-pass-warn')
    };
  }

  const dom = lookupDom();
  const state = { plaintext: null, files: [], urls: [], pending: 0, busy: false, locked: false };
  state.openLabel = dom.openLabel ? dom.openLabel.textContent : '';
  const ctx = { util: window.goneUtil, icons: window.goneIcons, formatBytes: window.goneFileMeta.formatBytes, byId: byId, dom: dom, state: state, pass: null };

  function setStatus(msg) {
    ctx.util.setText(dom.status, msg);
  }

  function syncOpen() {
    const missing = ctx.pass ? ctx.pass.missing() : false;
    if (dom.open) dom.open.setAttribute('aria-disabled', String(state.busy || state.locked || missing));
  }

  ctx.setStatus = setStatus;
  ctx.syncOpen = syncOpen;
  window.goneConsumeViewBase = ctx;
})();
