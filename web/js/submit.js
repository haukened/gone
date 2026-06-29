'use strict';

// Submission flow & result panel extracted from app.js
(function submitFlow() {
  const form = document.getElementById('create-secret');
  if (!form || !window.goneCrypto) return;

  const textarea = document.getElementById('secret');
  const modeText = document.getElementById('mode-text');
  const modeFile = document.getElementById('mode-file');
  const fileField = document.getElementById('file-field');
  const fileInput = document.getElementById('secret-file');
  const fileSummary = document.getElementById('file-summary');
  const clearFileBtn = document.getElementById('clear-file');
  const ttlSelect = document.getElementById('ttl');
  const primaryBtn = form.querySelector('button[type="submit"]');
  const cardSection = form.closest('.card');
  const errorBox = document.getElementById('submit-error');
  const errorContent = document.getElementById('submit-error-content');
  if (!textarea || !ttlSelect || !primaryBtn || !cardSection) return;

  const FILE_MAGIC = new TextEncoder().encode('GONEFILE1');
  const GCM_TAG_BYTES = 16;
  const maxBytes = parseInt(form.dataset.maxBytes || '0', 10) || 0;

  const debugTiming = (function(){
    try {
      const params = new URLSearchParams(location.search);
      if (params.get('debug') === 'timing') return true;
      return (window.localStorage && localStorage.getItem('goneDebugTiming') === '1');
    } catch(_) { return false; }
  })();

  function logTiming(label, start, end) {
    if (!debugTiming) return;
    console.log(`[gone][timing] ${label}: ${(end - start).toFixed(2)}ms`);
  }

  // We deliberately avoid mutating button innerHTML dynamically to reduce XSS surface.
  // Progress is indicated solely via disabled state + aria-busy attribute.

  function showError(msg) {
    if (!errorBox) return;
    errorContent.textContent = msg;
    errorBox.hidden = false;
    errorBox.setAttribute('aria-hidden', 'false');
  }

  function secureWipe(raw) {
    try {
      const len = raw.length;
      textarea.value = ''.padEnd(len, '\u2022');
      textarea.value = '';
    } catch (_) {
      // best-effort
    }
  }

  function clearError() {
    if (errorBox) {
      errorBox.hidden = true;
      errorBox.setAttribute('aria-hidden', 'true');
    }
  }

  function formatBytes(bytes) {
    if (!Number.isFinite(bytes) || bytes < 0) return '0 B';
    const units = ['B', 'KiB', 'MiB', 'GiB'];
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
      value /= 1024;
      unit++;
    }
    const digits = unit === 0 ? 0 : value < 10 ? 1 : 0;
    return `${value.toFixed(digits)} ${units[unit]}`;
  }

  function sanitizeFileName(name) {
    const parts = String(name || '').split(/[\\/]+/).filter(Boolean);
    const base = (parts.length ? parts[parts.length - 1] : '').replace(/[\x00-\x1f\x7f]/g, '').trim();
    if (!base || base === '.' || base === '..') return 'download.bin';
    return base;
  }

  function getSelectedMode() {
    return modeFile && modeFile.checked ? 'file' : 'text';
  }

  function selectedFile() {
    return fileInput && fileInput.files && fileInput.files.length ? fileInput.files[0] : null;
  }

  function updateModeUI() {
    const fileMode = getSelectedMode() === 'file';
    textarea.hidden = fileMode;
    textarea.disabled = fileMode;
    if (fileField) fileField.hidden = !fileMode;
    if (fileInput) fileInput.disabled = !fileMode;
    if (!fileMode) textarea.focus();
  }

  function updateFileSummary() {
    const file = selectedFile();
    if (!file) {
      if (fileSummary) fileSummary.textContent = 'No file selected';
      if (clearFileBtn) clearFileBtn.hidden = true;
      return;
    }
    const name = sanitizeFileName(file.name);
    const type = file.type || 'application/octet-stream';
    if (fileSummary) fileSummary.textContent = `${name} - ${formatBytes(file.size)} - ${type}`;
    if (clearFileBtn) clearFileBtn.hidden = false;
  }

  function estimateCiphertextBytes(plaintextBytes) {
    return plaintextBytes + GCM_TAG_BYTES;
  }

  async function buildFileEnvelope(file) {
    const name = sanitizeFileName(file.name);
    const type = file.type || 'application/octet-stream';
    const metadata = { kind: 'file', name, type, size: file.size };
    const metadataBytes = new TextEncoder().encode(JSON.stringify(metadata));
    const envelopeSize = FILE_MAGIC.length + 4 + metadataBytes.length + file.size;
    if (maxBytes && estimateCiphertextBytes(envelopeSize) > maxBytes) {
      throw new Error('file too large');
    }
    const fileBytes = new Uint8Array(await file.arrayBuffer());
    const envelope = new Uint8Array(FILE_MAGIC.length + 4 + metadataBytes.length + fileBytes.length);
    let offset = 0;
    envelope.set(FILE_MAGIC, offset);
    offset += FILE_MAGIC.length;
    envelope[offset++] = (metadataBytes.length >>> 24) & 0xff;
    envelope[offset++] = (metadataBytes.length >>> 16) & 0xff;
    envelope[offset++] = (metadataBytes.length >>> 8) & 0xff;
    envelope[offset++] = metadataBytes.length & 0xff;
    envelope.set(metadataBytes, offset);
    offset += metadataBytes.length;
    envelope.set(fileBytes, offset);
    return envelope;
  }

  async function encryptSecret(raw) {
    const key = window.goneCrypto.generateKey();
    const encStart = performance.now();
    const encResult = await window.goneCrypto.encrypt(raw, key);
    const encEnd = performance.now();
    logTiming('encrypt', encStart, encEnd);
    return { key, encResult };
  }

  async function uploadCiphertext(encResult, keyBytes, ttl) {
    const { nonce, ciphertext } = encResult;
    const version = window.goneCrypto.version;
    const nonceB64 = window.goneCrypto.b64urlEncode(nonce);
    const uploadStart = performance.now();
    primaryBtn.setAttribute('aria-busy', 'true');
    const resp = await fetch('/api/secret', {
      method: 'POST',
      headers: {
        'X-Gone-Version': String(version),
        'X-Gone-Nonce': nonceB64,
        'X-Gone-TTL': ttl,
        'Content-Type': 'application/octet-stream'
      },
      body: ciphertext
    });
    const uploadEnd = performance.now();
    logTiming('upload', uploadStart, uploadEnd);
    if (!resp.ok) {
      console.error('[gone] server error', resp.status);
      showError(resp.status === 413 ? 'Secret too large' : 'Server error creating secret');
      return null;
    }
    return { json: await resp.json(), version, keyBytes };
  }

  function buildShareURL(id, version, keyBytes) {
    const keyB64 = window.goneCrypto.exportKeyB64(keyBytes);
    return `${location.origin}/secret/${id}#v${version}:${keyB64}`;
  }

  function resetButton() {
    primaryBtn.disabled = false;
    primaryBtn.removeAttribute('aria-busy');
  }

  function failureDelayReset(delay) {
    setTimeout(function () {
      resetButton();
    }, delay);
  }

  function logTotal(start) {
    const t1 = performance.now();
    logTiming('total_submit_cycle', start, t1);
  }

  function prepareSubmission() {
    const mode = getSelectedMode();
    const ttl = ttlSelect.value;
    const raw = textarea.value;
    let file = null;
    if (mode === 'file') {
      file = selectedFile();
      if (!file) {
        console.warn('[gone] empty file submission blocked');
        showError('Choose a file before encrypting');
        return null;
      }
    } else if (!raw) {
      console.warn('[gone] empty secret submission blocked');
      showError('Cannot submit empty secret');
      return null;
    }
    primaryBtn.disabled = true;
    clearError();
    return { mode, raw, file, ttl, t0: performance.now() };
  }

  async function performEncryption(raw) {
    try {
      const { key, encResult } = await encryptSecret(raw);
      return { keyBytes: key, encResult };
    } catch (e) {
      console.error('[gone] encryption failed', e);
      showError('Encryption failed');
      resetButton();
      return null;
    }
  }

  async function performUpload(encResult, keyBytes, ttl) {
    try {
      const res = await uploadCiphertext(encResult, keyBytes, ttl);
      if (!res) failureDelayReset(1500);
      return res;
    } catch (e) {
      console.error('[gone] upload failed', e);
      showError('Network error uploading secret');
      failureDelayReset(1500);
      return null;
    }
  }

  function finalizeSubmission(uploadRes, keyBytes, t0) {
    logTotal(t0);
    const secretID = uploadRes.json.id;
    if (!secretID) {
      console.error('[gone] missing id in response payload', uploadRes.json);
      showError('Unexpected server response');
      failureDelayReset(1500);
      return;
    }
    const shareURL = buildShareURL(secretID, uploadRes.version, keyBytes);
    buildAndShowResultPanel({
      shareURL,
      expiresAt: uploadRes.json.expires_at,
      replaceTarget: cardSection
    });
    resetButton();
  }

  async function runSubmission(prep) {
    let payload = prep.raw;
    try {
      if (prep.mode === 'file') payload = await buildFileEnvelope(prep.file);
    } catch (e) {
      console.warn('[gone] file payload rejected', e);
      showError(e && e.message === 'file too large' ? 'File too large for this server limit' : 'File could not be read');
      resetButton();
      return;
    }
    const encryption = await performEncryption(payload);
    if (!encryption) return;
    if (prep.mode === 'text') secureWipe(prep.raw);
    if (prep.mode === 'file' && fileInput) fileInput.value = '';
    updateFileSummary();
    const uploadRes = await performUpload(encryption.encResult, encryption.keyBytes, prep.ttl);
    if (!uploadRes) return;
    finalizeSubmission(uploadRes, encryption.keyBytes, prep.t0);
  }

  function handleSubmit(ev) {
    ev.preventDefault();
    const prep = prepareSubmission();
    if (!prep) return;
    // Fire and forget; internal helpers handle errors & UI state.
    runSubmission(prep);
  }

  form.addEventListener('submit', handleSubmit);
  if (modeText) modeText.addEventListener('change', updateModeUI);
  if (modeFile) modeFile.addEventListener('change', updateModeUI);
  if (fileInput) fileInput.addEventListener('change', updateFileSummary);
  if (clearFileBtn) {
    clearFileBtn.addEventListener('click', function () {
      if (fileInput) fileInput.value = '';
      updateFileSummary();
      if (fileInput) fileInput.focus();
    });
  }
  updateModeUI();
  updateFileSummary();

  (function previewCheck() {
    const params = new URLSearchParams(location.search);
    if (params.get('preview') === 'result') {
      const mockID = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
      const mockKey = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';
      const version = window.goneCrypto.version;
      const mockURL = `${location.origin}/secret/${mockID}#v${version}:${mockKey}`;
      const future = new Date(Date.now() + 30 * 60 * 1000).toISOString();
      buildAndShowResultPanel({
        shareURL: mockURL,
        expiresAt: future,
        replaceTarget: form.closest('.card'),
        focus: false
      });
    }
  })();
})();

function buildAndShowResultPanel(opts) {
  const { shareURL, expiresAt, replaceTarget, focus = true } = opts;
  const BACK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-left-icon lucide-arrow-left"><path d="m12 19-7-7 7-7"/><path d="M19 12H5"/></svg>';
  const COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';
  const CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
  const panel = document.createElement('div');
  const outer = document.createElement('div'); outer.id = 'result-outer'; panel.appendChild(outer);
  const h2 = document.createElement('h2'); h2.className = 'underline'; h2.textContent = 'Share This Link'; outer.appendChild(h2);
  const warnP = document.createElement('p'); warnP.className = 'security-warning-card'; warnP.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4"/><path d="M12 17h.01"/><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/></svg>';
  const warnSpan = document.createElement('span'); warnSpan.textContent = 'Anyone with this link can view the secret exactly once.'; warnP.appendChild(warnSpan); outer.appendChild(warnP);
  const card = document.createElement('div'); card.className = 'card'; outer.appendChild(card);
  const hintP = document.createElement('p'); hintP.className = 'hint'; card.appendChild(hintP);
  const span = document.createElement('span'); hintP.appendChild(span); span.appendChild(document.createTextNode('Expires at '));
  const timeEl = document.createElement('time'); timeEl.setAttribute('datetime', expiresAt); timeEl.textContent = new Date(expiresAt).toLocaleString(); span.appendChild(timeEl);
  const input = document.createElement('input'); input.className = 'share-link'; input.id = 'share-link'; input.type = 'text'; input.readOnly = true; input.value = shareURL; card.appendChild(input);
  const actions = document.createElement('div'); actions.className = 'result-actions'; card.appendChild(actions);
  const backLink = document.createElement('a'); backLink.href = '/'; backLink.className = 'back-link'; backLink.innerHTML = BACK_ICON + ' Create Another'; actions.appendChild(backLink);
  const copyBtn = document.createElement('button'); copyBtn.type = 'button'; copyBtn.className = 'copy-primary-btn'; copyBtn.setAttribute('aria-label', 'Copy full share link'); copyBtn.innerHTML = 'Copy Link ' + COPY_ICON; actions.appendChild(copyBtn);
  if (replaceTarget) replaceTarget.replaceWith(panel); else document.body.appendChild(panel);
  const shareInput = panel.querySelector('#share-link');
  const copyBtnRoot = panel.querySelector('.copy-primary-btn');
  if (focus && shareInput) {
    shareInput.focus();
    // Ensure the beginning of the long URL is visible. Some browsers scroll to the end on focus.
    try {
      // Defer to next frame so layout/selection are applied after focus.
      requestAnimationFrame(function() {
        shareInput.selectionStart = 0;
        shareInput.selectionEnd = 0;
        shareInput.scrollLeft = 0;
      });
    } catch (_) { /* non-critical */ }
  }
  if (copyBtnRoot) { copyBtnRoot.addEventListener('click', async function () { let success = true; try { await navigator.clipboard.writeText(shareURL); } catch (_) { if (shareInput) { shareInput.focus(); shareInput.select(); } success = false; } if (success) { copyBtnRoot.innerHTML = 'Copied! ' + CHECK_ICON; copyBtnRoot.classList.add('copied'); copyBtnRoot.disabled = true; setTimeout(function () { copyBtnRoot.innerHTML = 'Copy Link ' + COPY_ICON; copyBtnRoot.classList.remove('copied'); copyBtnRoot.disabled = false; }, 2200); } else { alert('Copy failed. Please press \u2318/Ctrl+C to copy manually.'); } return success; }); }
  console.log('[gone] result panel shown');
}
