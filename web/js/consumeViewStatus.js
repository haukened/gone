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
      ctx.setStatus('js.consume.retrievingPct', { pct: Math.floor((received / total) * 100) });
      return;
    }
    bar.removeAttribute('value');
    ctx.setStatus('js.consume.retrievingSize', { size: { bytes: received } });
  }

  function hideProgress() {
    if (dom.progress) dom.progress.hidden = true;
  }

  // showError shows an error message key.
  function showError(key) {
    hideProgress();
    ctx.setStatus('');
    ctx.i18n.set(dom.errorText, key);
    if (dom.errorBox) dom.errorBox.hidden = false;
  }

  function setOpening(busy) {
    state.busy = busy;
    if (dom.pass) dom.pass.readOnly = busy || state.locked;
    ctx.syncOpen();
    if (dom.open && busy) dom.open.setAttribute('aria-busy', 'true');
    else if (dom.open) dom.open.removeAttribute('aria-busy');
    if (busy) ctx.i18n.set(dom.openLabel, 'js.consume.opening');
    else ctx.i18n.restore(dom.openLabel, state.openLabel);
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
