'use strict';

// Final consume view operations that combine status, message, and file modules.
(function consumeViewFinalModule() {
  if (window.goneConsumeViewFinal || !window.goneConsumeViewBase || !window.goneConsumeViewStatus || !window.goneConsumeViewMessages || !window.goneConsumeViewFiles) return;
  const ctx = window.goneConsumeViewBase;
  const dom = ctx.dom;
  const state = ctx.state;
  const status = window.goneConsumeViewStatus;
  const messages = window.goneConsumeViewMessages;
  const files = window.goneConsumeViewFiles;

  function headingFor(decoded) {
    const n = decoded.files.length;
    const label = n === 1 ? 'a file' : `${n} files`;
    if (decoded.message && n) return `Here\u2019s your secret and ${label}.`;
    if (n) return `Here\u2019s ${label}.`;
    return 'Here\u2019s your secret.';
  }

  function showDecoded(decoded) {
    status.hideProgress();
    ctx.pass.clear();
    ctx.util.setText(dom.revealedHeading, headingFor(decoded));
    messages.showMessage(decoded.message);
    files.showFiles(decoded.files);
    messages.switchTo('revealed');
  }

  function cleanup() {
    ctx.pass.clear();
    state.urls.forEach(function (u) { URL.revokeObjectURL(u); });
    state.urls = [];
    if (state.plaintext) state.plaintext.fill(0);
  }

  window.goneConsumeViewFinal = Object.freeze({
    headingFor: headingFor,
    showDecoded: showDecoded,
    showGone: function () { messages.switchTo('gone'); },
    showAckResult: function (ok) { if (!ok && dom.ackWarning) dom.ackWarning.hidden = false; },
    guardUnload: function (ev) { if (state.pending <= 0) return; ev.preventDefault(); ev.returnValue = ''; },
    keepPlaintext: function (bytes) { state.plaintext = bytes; },
    cleanup: cleanup
  });
})();
