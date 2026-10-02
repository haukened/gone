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

  const textarea = byId('secret');
  const ttlSelect = byId('ttl');
  const uploadProgress = byId('upload-progress');
  const errorBox = byId('submit-error');
  const errorContent = byId('submit-error-content');
  const primaryBtn = form.querySelector('button[type="submit"]');
  const primaryLabel = primaryBtn ? primaryBtn.querySelector('span') : null;
  const cardSection = form.closest('.card');
  if (!util.allPresent([textarea, ttlSelect, primaryBtn, cardSection])) return;

  const meterEls = { meter: byId('size-meter'), label: byId('size-label'), warning: byId('size-warning') };
  const maxBytes = parsePositiveInt(form.dataset.maxBytes);
  const idleLabel = labelText(primaryLabel, 'Encrypt');
  const fileInput = byId('secret-files');
  let busy = false;

  const selection = window.goneFileSelection.create({
    form: form,
    input: fileInput,
    dropZone: byId('drop-zone'),
    listEl: byId('file-list'),
    isBusy: function () { return busy; },
    onAdd: clearError,
    onChange: updateMeter
  });

  function setErrorVisible(visible) {
    if (!errorBox) return;
    errorBox.hidden = !visible;
    errorBox.setAttribute('aria-hidden', String(!visible));
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
    } catch (_) {
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

  function updateMeter() {
    const empty = isEmpty();
    const size = empty ? 0 : envelope.encryptedSize(textarea.value, selection.metas());
    const problem = currentProblem(size);
    sizeMeter.render(meterEls, size, maxBytes, problem);
    primaryBtn.disabled = busy || empty || Boolean(problem);
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
    primaryBtn.toggleAttribute('aria-busy', state);
    textarea.readOnly = state;
    if (fileInput) fileInput.disabled = state;
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

  async function encryptCurrent(message) {
    setButtonLabel('Encrypting\u2026');
    try {
      return await uploader.encryptSelection(message, selection.files());
    } catch (e) {
      failSubmission('[gone] encryption failed', e, 'Encryption failed: a file could not be read.');
      return null;
    }
  }

  function showResult(json, keyBytes) {
    window.goneResultPanel.show({
      shareURL: uploader.buildShareURL(json.id, keyBytes),
      expiresAt: json.expires_at,
      replaceTarget: cardSection
    });
  }

  async function runSubmission() {
    const t0 = performance.now();
    const message = textarea.value;
    const ttl = ttlSelect.value;
    const enc = await encryptCurrent(message);
    if (!enc) return;
    try {
      const json = await uploader.upload(enc.encResult, ttl, setUploadProgress);
      secureWipe(message);
      selection.clear();
      util.logTiming('total_submit_cycle', t0, performance.now());
      showResult(json, enc.keyBytes);
    } catch (e) {
      failSubmission('[gone] upload failed', e, uploader.friendlyError(e));
    } finally {
      enc.keyBytes.fill(0);
    }
  }

  function submitProblem() {
    if (isEmpty()) return 'Add a message or at least one file';
    return currentProblem(envelope.encryptedSize(textarea.value, selection.metas()));
  }

  function handleSubmit(ev) {
    ev.preventDefault();
    if (busy) return;
    const problem = submitProblem();
    if (problem) {
      showError(problem);
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
    const mockURL = `${location.origin}/secret/${mockID}#v${window.goneCrypto.version}:${mockKey}`;
    const future = new Date(Date.now() + 30 * 60 * 1000).toISOString();
    window.goneResultPanel.show({ shareURL: mockURL, expiresAt: future, replaceTarget: cardSection, focus: false });
  }

  form.addEventListener('submit', handleSubmit);
  textarea.addEventListener('input', updateMeter);
  selection.render();
  previewResult();
})();
