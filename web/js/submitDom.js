'use strict';

// Shared DOM lookup and dependency checks for the submit flow.
(function submitDomModule() {
  if (window.goneSubmitDom) return;
  const util = window.goneUtil;
  const form = document.getElementById('create-secret');
  // The result panel is only needed by the default (send) target; the reply
  // page supplies its own window.goneSubmitTarget.
  const deps = [window.goneEnvelope, window.goneFileSelection, window.goneSizeMeter, window.goneUpload, window.goneSubmitTarget || window.goneResultPanel];
  if (!util || !util.allPresent([form].concat(deps))) return;

  function byId(id) {
    return document.getElementById(id);
  }

  function parsePositiveInt(raw) {
    const n = parseInt(raw || '0', 10);
    return n > 0 ? n : 0;
  }



  function lookupElements() {
    const btn = form.querySelector('button[type="submit"]');
    return {
      textarea: byId('secret'),
      uploadProgress: byId('upload-progress'),
      errorBox: byId('submit-error'),
      errorContent: byId('submit-error-content'),
      primaryBtn: btn,
      primaryLabel: btn ? btn.querySelector('span') : null,
      meterEls: { box: byId('size-box'), meter: byId('size-meter'), label: byId('size-label'), warning: byId('size-warning'), warningText: byId('size-warning-text') }
    };
  }

  const els = lookupElements();
  if (!util.allPresent([els.textarea, els.primaryBtn])) return;
  window.goneSubmitDom = {
    util: util,
    form: form,
    byId: byId,
    envelope: window.goneEnvelope,
    sizeMeter: window.goneSizeMeter,
    uploader: window.goneUpload,
    maxBytes: parsePositiveInt(form.dataset.maxBytes),
    overhead: parsePositiveInt(form.dataset.overhead),
    idleLabel: window.goneI18n ? window.goneI18n.snapshot(els.primaryLabel) : null,
    fileInput: byId('secret-files'),
    busy: false,
    els: els
  };
})();
