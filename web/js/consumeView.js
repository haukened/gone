'use strict';

// Public DOM side of the consume flow.
(function consumeViewModule() {
  if (window.goneConsumeView) return;
  const ctx = window.goneConsumeViewBase;
  const status = window.goneConsumeViewStatus;
  const messages = window.goneConsumeViewMessages;
  const finalView = window.goneConsumeViewFinal;
  if (!ctx || !ctx.pass || !status || !messages || !finalView) return;

  window.goneConsumeView = Object.freeze({
    present: Boolean(ctx.byId('view-open')),
    setStatus: status.setStatus,
    setProgress: status.setProgress,
    hideProgress: status.hideProgress,
    showError: status.showError,
    clearError: status.clearError,
    setOpening: status.setOpening,
    disableOpen: status.disableOpen,
    onOpen: messages.onOpen,
    showPassphrase: ctx.pass.show,
    passphrase: ctx.pass.value,
    passphraseMissing: ctx.pass.missing,
    focusPassphrase: ctx.pass.focus,
    passphraseFailed: ctx.pass.failed,
    clearPassphrase: ctx.pass.clear,
    showMessage: messages.showMessage,
    showDecoded: finalView.showDecoded,
    showGone: finalView.showGone,
    showAckResult: finalView.showAckResult,
    guardUnload: finalView.guardUnload,
    keepPlaintext: finalView.keepPlaintext,
    cleanup: finalView.cleanup,
    headingFor: finalView.headingFor
  });
})();
