'use strict';

// Status, progress, and Open button state for the consume view.
(function consumeViewStatusModule() {
  if (window.goneConsumeViewStatus || !window.goneConsumeViewBase) return;
  const ctx = window.goneConsumeViewBase;
  const dom = ctx.dom;
  const state = ctx.state;

  function setProgress(received, total) {
    const bar = dom.progress;
    if (!bar) return;
    bar.hidden = false;
    if (total > 0) {
      bar.max = total;
      bar.value = Math.min(received, total);
      ctx.setStatus(`Retrieving\u2026 ${Math.floor((received / total) * 100)}%`);
      return;
    }
    bar.removeAttribute('value');
    ctx.setStatus(`Retrieving\u2026 ${ctx.formatBytes(received)}`);
  }

  function hideProgress() {
    if (dom.progress) dom.progress.hidden = true;
  }

  function showError(msg) {
    hideProgress();
    ctx.setStatus('');
    ctx.util.setText(dom.errorText, msg);
    if (dom.errorBox) dom.errorBox.hidden = false;
  }

  function setOpening(busy) {
    state.busy = busy;
    if (dom.pass) dom.pass.readOnly = busy || state.locked;
    ctx.syncOpen();
    if (dom.open && busy) dom.open.setAttribute('aria-busy', 'true');
    else if (dom.open) dom.open.removeAttribute('aria-busy');
    ctx.util.setText(dom.openLabel, busy ? 'Opening\u2026' : state.openLabel);
  }

  function disableOpen() {
    state.locked = true;
    if (dom.pass) dom.pass.readOnly = true;
    ctx.syncOpen();
  }

  window.goneConsumeViewStatus = Object.freeze({
    setStatus: ctx.setStatus,
    setProgress: setProgress,
    hideProgress: hideProgress,
    showError: showError,
    clearError: function () { if (dom.errorBox) dom.errorBox.hidden = true; },
    setOpening: setOpening,
    disableOpen: disableOpen
  });
})();
