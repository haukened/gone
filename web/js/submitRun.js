'use strict';

// Submit flow actions: validate, encrypt, upload, and render the result.
(function submitRunModule() {
  if (window.goneSubmitRun || !window.goneSubmitDom || !window.goneSubmitState || !window.goneSubmitUi) return;
  const ctx = window.goneSubmitDom;
  const state = window.goneSubmitState;
  const ui = window.goneSubmitUi;

  function selectedTTL() {
    const ttl = ctx.form.elements.namedItem('ttl');
    return ttl ? ttl.value : '';
  }

  const NO_CRYPTO = 'This browser only encrypts on secure (HTTPS) pages, so Gone can\u2019t encrypt here. Ask whoever runs this server to enable HTTPS.';

  // sendTarget is the default target: a new secret under a fresh key
  // (v1, or v2 with a passphrase), shown in the result panel. A page can
  // define window.goneSubmitTarget with the same {encrypt, upload, show,
  // wipe} shape to send the form somewhere else.
  const sendTarget = {
    encrypt: function (message, files, pass) { return ctx.uploader.encryptSelection(message, files, pass); },
    upload: function (enc, onProgress) { return ctx.uploader.upload(enc.encResult, selectedTTL(), onProgress, enc.version); },
    show: function (json, enc) {
      window.goneResultPanel.show({
        shareURL: ctx.uploader.buildShareURL(json.id, enc.keyBytes, enc.version),
        manageURL: ctx.uploader.buildManageURL(json.id, json.manage_token),
        expiresAt: json.expires_at,
        passphrase: enc.version !== window.goneCrypto.version
      });
    },
    wipe: function (enc) { enc.keyBytes.fill(0); }
  };
  const target = window.goneSubmitTarget || sendTarget;

  async function encryptCurrent(message, pass) {
    if (!window.goneUtil.cryptoAvailable()) {
      ui.failSubmission('[gone] WebCrypto unavailable (insecure context)', new Error('crypto.subtle missing'), NO_CRYPTO);
      return null;
    }
    ui.setButtonLabel('Encrypting\u2026');
    try {
      return await target.encrypt(message, state.selection.files(), pass);
    } catch (e) {
      ui.failSubmission('[gone] encryption failed', e, (e && e.userMessage) || 'Encryption failed: a file could not be read.');
      return null;
    }
  }

  async function runSubmission() {
    const t0 = performance.now();
    const message = ctx.els.textarea.value;
    const enc = await encryptCurrent(message, state.passValue());
    if (!enc) return;
    try {
      const json = await target.upload(enc, ui.setUploadProgress);
      ui.secureWipe(message);
      state.selection.clear();
      if (state.passphrase) state.passphrase.clear();
      ctx.util.logTiming('total_submit_cycle', t0, performance.now());
      target.show(json, enc);
    } catch (e) {
      ui.failSubmission('[gone] upload failed', e, ctx.uploader.friendlyError(e));
    } finally {
      target.wipe(enc);
    }
  }

  function submitProblem() {
    if (state.isEmpty()) return 'Add a message or at least one file.';
    return state.currentProblem(state.currentSize());
  }

  function handleSubmit(ev) {
    ev.preventDefault();
    if (ctx.busy) return;
    const problem = submitProblem();
    if (problem) {
      ui.showError(problem);
      return;
    }
    const pass = state.passProblem();
    if (pass) {
      ui.showError(pass);
      state.passphrase.reveal();
      return;
    }
    ui.clearError();
    ui.setBusy(true);
    runSubmission();
  }

  window.goneSubmitRun = { handleSubmit: handleSubmit, runSubmission: runSubmission };
})();
