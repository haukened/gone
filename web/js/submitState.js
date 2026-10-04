'use strict';

// Input selection, passphrase, and size calculations for submit.
(function submitStateModule() {
  if (window.goneSubmitState || !window.goneSubmitDom) return;
  const ctx = window.goneSubmitDom;

  function clearError() {
    if (window.goneSubmitUi) window.goneSubmitUi.clearError();
  }

  function createPassphrase() {
    if (!window.gonePassphraseField) return null;
    return window.gonePassphraseField.create({
      disclosure: ctx.byId('pass-disclosure'),
      input: ctx.byId('passphrase'),
      toggle: ctx.byId('pass-toggle'),
      generate: ctx.byId('pass-generate'),
      strength: ctx.byId('pass-strength')
    }, function () { clearError(); updateMeter(); });
  }

  function createSelection() {
    return window.goneFileSelection.create({
      form: ctx.form,
      input: ctx.fileInput,
      dropZone: ctx.byId('drop-zone'),
      listEl: ctx.byId('file-list'),
      isBusy: function () { return ctx.busy; },
      onAdd: clearError,
      onChange: updateMeter
    });
  }

  const selection = createSelection();
  const passphrase = createPassphrase();

  function passValue() {
    return passphrase ? passphrase.value() : '';
  }

  function passOverhead() {
    return passphrase ? passphrase.overhead() : 0;
  }

  function passProblem() {
    return passphrase ? passphrase.problem() : '';
  }

  function isEmpty() {
    return !ctx.els.textarea.value && selection.count() === 0;
  }

  function currentSize() {
    return ctx.envelope.encryptedSize(ctx.els.textarea.value, selection.metas(), passOverhead());
  }

  function currentProblem(size) {
    return ctx.sizeMeter.selectionProblem(selection.count(), size, ctx.envelope.MAX_FILES, ctx.maxBytes);
  }

  function updateMeter() {
    const empty = isEmpty();
    const size = empty ? 0 : currentSize();
    const problem = currentProblem(size);
    ctx.sizeMeter.render(ctx.els.meterEls, size, ctx.maxBytes, problem);
    ctx.els.primaryBtn.setAttribute('aria-disabled', String(ctx.busy || empty || Boolean(problem || passProblem())));
  }

  window.goneSubmitState = { selection: selection, passphrase: passphrase, passValue: passValue, passProblem: passProblem, isEmpty: isEmpty, currentSize: currentSize, currentProblem: currentProblem, updateMeter: updateMeter };
})();
