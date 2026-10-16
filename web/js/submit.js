'use strict';

// Secret creation flow wiring: message + optional files -> encrypt -> upload.
(function submitFlow() {
  if (!window.goneSubmitDom || !window.goneSubmitState || !window.goneSubmitRun) return;
  const ctx = window.goneSubmitDom;
  const state = window.goneSubmitState;

  // Without WebCrypto (plain HTTP) nothing can be encrypted, so the button
  // stays off and the hint says why.
  if (!ctx.util.cryptoAvailable()) {
    ctx.els.primaryBtn.disabled = true;
    window.goneI18n.set(ctx.byId('submit-hint'), 'js.submit.noCryptoHint');
  }
  ctx.form.addEventListener('submit', window.goneSubmitRun.handleSubmit);
  ctx.els.textarea.addEventListener('input', state.updateMeter);
  state.selection.render();
  // Development builds only: web/dev/js/preview.js defines the hook.
  if (window.gonePreview) window.gonePreview.submit(ctx);
})();
