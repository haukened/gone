'use strict';

// UI updates for submit errors, progress, and busy state.
(function submitUiModule() {
  if (window.goneSubmitUi || !window.goneSubmitDom || !window.goneSubmitState) return;
  const ctx = window.goneSubmitDom;
  const state = window.goneSubmitState;
  const els = ctx.els;

  function setErrorVisible(visible) {
    if (els.errorBox) els.errorBox.hidden = !visible;
  }

  function showError(msg) {
    ctx.util.setText(els.errorContent, msg);
    setErrorVisible(true);
  }

  function clearError() {
    setErrorVisible(false);
  }

  function secureWipe(raw) {
    try {
      els.textarea.value = ''.padEnd(raw.length, '\u2022');
      els.textarea.value = '';
    } catch {
      // best-effort
    }
  }

  function setButtonLabel(text) {
    ctx.util.setText(els.primaryLabel, text);
  }

  function setUploadProgress(loaded, total) {
    const pct = total ? Math.floor((loaded / total) * 100) : 0;
    setButtonLabel(`Uploading ${pct}%`);
    if (!els.uploadProgress) return;
    els.uploadProgress.hidden = false;
    els.uploadProgress.max = total || 1;
    els.uploadProgress.value = loaded;
    els.uploadProgress.textContent = `${pct}%`;
  }

  function setBusy(busy) {
    ctx.busy = busy;
    if (busy) els.primaryBtn.setAttribute('aria-busy', 'true');
    else els.primaryBtn.removeAttribute('aria-busy');
    els.textarea.readOnly = busy;
    if (ctx.fileInput) ctx.fileInput.disabled = busy;
    if (state.passphrase) state.passphrase.setBusy(busy);
    if (!busy) {
      setButtonLabel(ctx.idleLabel);
      if (els.uploadProgress) els.uploadProgress.hidden = true;
    }
    state.updateMeter();
  }

  function failSubmission(logMsg, err, userMsg) {
    console.error(logMsg, err);
    showError(userMsg);
    setBusy(false);
  }

  window.goneSubmitUi = { showError: showError, clearError: clearError, secureWipe: secureWipe, setButtonLabel: setButtonLabel, setUploadProgress: setUploadProgress, setBusy: setBusy, failSubmission: failSubmission };
})();
