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

  // headingFor returns the revealed heading as a message key and args.
  function headingFor(decoded) {
    const n = decoded.files.length;
    if (decoded.message && n) return { key: 'js.consume.headingSecretFiles', args: { count: n } };
    if (n) return { key: 'js.consume.headingFiles', args: { count: n } };
    return { key: 'secret.revealedTitle' };
  }

  function showDecoded(decoded) {
    status.hideProgress();
    ctx.pass.clear();
    const heading = headingFor(decoded);
    ctx.i18n.set(dom.revealedHeading, heading.key, heading.args);
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
