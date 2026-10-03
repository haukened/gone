'use strict';

// Secret creation flow: message + optional files -> envelope -> AES-GCM -> upload.
// File selection, the size meter, upload, and the result panel live in
// submitFiles.js, submitMeter.js, submitUpload.js and submitResult.js.
(function submitFlow() {
  const util = window.goneUtil;
  const form = document.getElementById('create-secret');
  const deps = [window.goneCrypto, window.goneEnvelope, window.goneFileSelection, window.goneSizeMeter, window.goneUpload, window.goneResultPanel];
  if (!util || !util.allPresent([form].concat(deps))) return;
  const envelope = window.goneEnvelope;
  const sizeMeter = window.goneSizeMeter;
  const uploader = window.goneUpload;

  function byId(id) {
    return document.getElementById(id);
  }

  // parsePositiveInt parses a positive integer attribute; anything else is 0.
  function parsePositiveInt(raw) {
    const n = parseInt(raw || '0', 10);
    return n > 0 ? n : 0;
  }

  function labelText(node, fallback) {
    return node ? node.textContent : fallback;
  }

  // lookupElements finds the form's elements; any may be null.
  function lookupElements() {
    const btn = form.querySelector('button[type="submit"]');
    return {
      textarea: byId('secret'),
      uploadProgress: byId('upload-progress'),
      errorBox: byId('submit-error'),
      errorContent: byId('submit-error-content'),
      primaryBtn: btn,
      primaryLabel: btn ? btn.querySelector('span') : null,
      meterEls: {
        box: byId('size-box'),
        meter: byId('size-meter'),
        label: byId('size-label'),
        warning: byId('size-warning'),
        warningText: byId('size-warning-text')
      }
    };
  }

  const { textarea, uploadProgress, errorBox, errorContent, primaryBtn, primaryLabel, meterEls } = lookupElements();
  if (!util.allPresent([textarea, primaryBtn])) return;

  const maxBytes = parsePositiveInt(form.dataset.maxBytes);
  const idleLabel = labelText(primaryLabel, 'Encrypt');
  const fileInput = byId('secret-files');
  let busy = false;

  const selection = createSelection();
  const passphrase = createPassphrase();

  // createPassphrase wires the optional passphrase field; null on pages without it.
  function createPassphrase() {
    if (!window.gonePassphraseField) return null;
    return window.gonePassphraseField.create({
      disclosure: byId('pass-disclosure'),
      input: byId('passphrase'),
      toggle: byId('pass-toggle'),
      generate: byId('pass-generate'),
      strength: byId('pass-strength')
    }, function () { clearError(); updateMeter(); });
  }

  function passValue() {
    return passphrase ? passphrase.value() : '';
  }

  function passOverhead() {
    return passphrase ? passphrase.overhead() : 0;
  }

  function passProblem() {
    return passphrase ? passphrase.problem() : '';
  }

  function createSelection() {
    return window.goneFileSelection.create({
      form: form,
      input: fileInput,
      dropZone: byId('drop-zone'),
      listEl: byId('file-list'),
      isBusy: function () { return busy; },
      onAdd: clearError,
      onChange: updateMeter
    });
  }

  function setErrorVisible(visible) {
    if (errorBox) errorBox.hidden = !visible;
  }

  // selectedTTL returns the checked TTL radio's value, or '' for the server default.
  function selectedTTL() {
    const ttl = form.elements.namedItem('ttl');
    return ttl ? ttl.value : '';
  }

  function showError(msg) {
    util.setText(errorContent, msg);
    setErrorVisible(true);
  }

  function clearError() {
    setErrorVisible(false);
  }

  function secureWipe(raw) {
    try {
      textarea.value = ''.padEnd(raw.length, '\u2022');
      textarea.value = '';
    } catch {
      // best-effort
    }
  }

  function setButtonLabel(text) {
    util.setText(primaryLabel, text);
  }

  function isEmpty() {
    return !textarea.value && selection.count() === 0;
  }

  function currentProblem(size) {
    return sizeMeter.selectionProblem(selection.count(), size, envelope.MAX_FILES, maxBytes);
  }

  function currentSize() {
    return envelope.encryptedSize(textarea.value, selection.metas(), passOverhead());
  }

  function updateMeter() {
    const empty = isEmpty();
    const size = empty ? 0 : currentSize();
    const problem = currentProblem(size);
    sizeMeter.render(meterEls, size, maxBytes, problem);
    // aria-disabled keeps the button focusable; pressing it explains what's missing.
    const blocked = busy || empty || Boolean(problem || passProblem());
    primaryBtn.setAttribute('aria-disabled', String(blocked));
  }

  function setUploadProgress(loaded, total) {
    const pct = total ? Math.floor((loaded / total) * 100) : 0;
    setButtonLabel(`Uploading ${pct}%`);
    if (!uploadProgress) return;
    uploadProgress.hidden = false;
    uploadProgress.max = total || 1;
    uploadProgress.value = loaded;
    uploadProgress.textContent = `${pct}%`;
  }

  function setBusy(state) {
    busy = state;
    // aria-busy must be the string "true"; an empty value means false.
    if (state) primaryBtn.setAttribute('aria-busy', 'true');
    else primaryBtn.removeAttribute('aria-busy');
    textarea.readOnly = state;
    if (fileInput) fileInput.disabled = state;
    if (passphrase) passphrase.setBusy(state);
    if (!state) {
      setButtonLabel(idleLabel);
      if (uploadProgress) uploadProgress.hidden = true;
    }
    updateMeter();
  }

  function failSubmission(logMsg, err, userMsg) {
    console.error(logMsg, err);
    showError(userMsg);
    setBusy(false);
  }

  async function encryptCurrent(message, pass) {
    setButtonLabel('Encrypting\u2026');
    try {
      return await uploader.encryptSelection(message, selection.files(), pass);
    } catch (e) {
      failSubmission('[gone] encryption failed', e, 'Encryption failed: a file could not be read.');
      return null;
    }
  }

  // showResult reveals the share link and, when the server issued one, the
  // sender's manage link built from json.manage_token.
  function showResult(json, enc) {
    window.goneResultPanel.show({
      shareURL: uploader.buildShareURL(json.id, enc.keyBytes, enc.version),
      manageURL: uploader.buildManageURL(json.id, json.manage_token),
      expiresAt: json.expires_at,
      passphrase: enc.version !== window.goneCrypto.version
    });
  }

  async function runSubmission() {
    const t0 = performance.now();
    const message = textarea.value;
    const ttl = selectedTTL();
    const enc = await encryptCurrent(message, passValue());
    if (!enc) return;
    try {
      const json = await uploader.upload(enc.encResult, ttl, setUploadProgress, enc.version);
      secureWipe(message);
      selection.clear();
      if (passphrase) passphrase.clear();
      util.logTiming('total_submit_cycle', t0, performance.now());
      showResult(json, enc);
    } catch (e) {
      failSubmission('[gone] upload failed', e, uploader.friendlyError(e));
    } finally {
      enc.keyBytes.fill(0);
    }
  }

  function submitProblem() {
    if (isEmpty()) return 'Add a message or at least one file.';
    return currentProblem(currentSize());
  }

  function handleSubmit(ev) {
    ev.preventDefault();
    if (busy) return;
    const problem = submitProblem();
    if (problem) {
      showError(problem);
      return;
    }
    const pass = passProblem();
    if (pass) {
      showError(pass);
      passphrase.reveal();
      return;
    }
    clearError();
    setBusy(true);
    runSubmission();
  }

  function previewResult() {
    if (new URLSearchParams(location.search).get('preview') !== 'result') return;
    const mockID = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
    const mockKey = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';
    const v2 = new URLSearchParams(location.search).has('passphrase');
    const version = v2 ? window.goneCrypto.versionV2 : window.goneCrypto.version;
    const mockURL = `${location.origin}/secret/${mockID}#v${version}:${mockKey}`;
    const manageURL = uploader.buildManageURL(mockID, 'B'.repeat(43));
    const future = new Date(Date.now() + 30 * 60 * 1000).toISOString();
    window.goneResultPanel.show({ shareURL: mockURL, manageURL: manageURL, expiresAt: future, passphrase: v2, focus: false });
  }

  form.addEventListener('submit', handleSubmit);
  textarea.addEventListener('input', updateMeter);
  selection.render();
  previewResult();
})();
