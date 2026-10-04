'use strict';

// Secret creation flow wiring: message + optional files -> encrypt -> upload.
(function submitFlow() {
  if (!window.goneSubmitDom || !window.goneSubmitState || !window.goneSubmitRun || !window.goneSubmitPreview) return;
  const ctx = window.goneSubmitDom;
  const state = window.goneSubmitState;

  ctx.form.addEventListener('submit', window.goneSubmitRun.handleSubmit);
  ctx.els.textarea.addEventListener('input', state.updateMeter);
  state.selection.render();
  window.goneSubmitPreview.previewResult();
})();
