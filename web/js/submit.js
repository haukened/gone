'use strict';

// Secret creation flow: message + optional files -> envelope -> AES-GCM -> upload.
(function submitFlow() {
  const form = document.getElementById('create-secret');
  if (!allPresent([form, window.goneCrypto, window.goneEnvelope])) return;
  const envelope = window.goneEnvelope;

  const textarea = document.getElementById('secret');
  const fileInput = document.getElementById('secret-files');
  const dropZone = document.getElementById('drop-zone');
  const fileList = document.getElementById('file-list');
  const sizeMeter = document.getElementById('size-meter');
  const sizeLabel = document.getElementById('size-label');
  const sizeWarning = document.getElementById('size-warning');
  const uploadProgress = document.getElementById('upload-progress');
  const ttlSelect = document.getElementById('ttl');
  const primaryBtn = form.querySelector('button[type="submit"]');
  const primaryLabel = primaryBtn && primaryBtn.querySelector('span');
  const cardSection = form.closest('.card');
  const errorBox = document.getElementById('submit-error');
  const errorContent = document.getElementById('submit-error-content');
  if (!allPresent([textarea, ttlSelect, primaryBtn, cardSection])) return;

  const maxBytes = parsePositiveInt(form.dataset.maxBytes);
  const idleLabel = primaryLabel ? primaryLabel.textContent : 'Encrypt';
  let selectedFiles = [];
  let busy = false;

  function allPresent(values) {
    return values.every(Boolean);
  }

  function parsePositiveInt(raw) {
    const n = parseInt(raw || '0', 10);
    return n > 0 ? n : 0;
  }

  const debugTiming = (function () {
    try {
      const params = new URLSearchParams(location.search);
      if (params.get('debug') === 'timing') return true;
      return (window.localStorage && localStorage.getItem('goneDebugTiming') === '1');
    } catch (_) { return false; }
  })();

  function logTiming(label, start, end) {
    if (!debugTiming) return;
    console.log(`[gone][timing] ${label}: ${(end - start).toFixed(2)}ms`);
  }

  function showError(msg) {
    if (!errorBox) return;
    errorContent.textContent = msg;
    errorBox.hidden = false;
    errorBox.setAttribute('aria-hidden', 'false');
  }

  function clearError() {
    if (!errorBox) return;
    errorBox.hidden = true;
    errorBox.setAttribute('aria-hidden', 'true');
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
    if (primaryLabel) primaryLabel.textContent = text;
  }

  // --- Selection & size meter --------------------------------------------
  function fileMetas() {
    return selectedFiles.map(function (f) { return { name: f.name, type: f.type, size: f.size }; });
  }

  function currentSize() {
    return envelope.encryptedSize(textarea.value, fileMetas());
  }

  function selectionProblem(size) {
    if (selectedFiles.length > envelope.MAX_FILES) {
      return `Too many files: remove ${selectedFiles.length - envelope.MAX_FILES} to stay within ${envelope.MAX_FILES}.`;
    }
    if (maxBytes && size > maxBytes) {
      return `Over the limit by ${envelope.formatBytes(size - maxBytes)}. Remove a file or shorten the message.`;
    }
    return '';
  }

  function renderMeterBar(size, problem) {
    if (!sizeMeter) return;
    sizeMeter.max = maxBytes || 1;
    sizeMeter.value = Math.min(size, sizeMeter.max);
    sizeMeter.classList.toggle('over', Boolean(problem));
  }

  function renderSizeLabel(size) {
    if (!sizeLabel) return;
    const over = maxBytes && size > maxBytes ? ' (over limit)' : '';
    sizeLabel.textContent = `${envelope.formatBytes(size)} of ${envelope.formatBytes(maxBytes)}${over}`;
  }

  function renderSizeWarning(problem) {
    if (!sizeWarning) return;
    sizeWarning.textContent = problem;
    sizeWarning.hidden = !problem;
  }

  function updateMeter() {
    const empty = !textarea.value && selectedFiles.length === 0;
    const size = empty ? 0 : currentSize();
    const problem = selectionProblem(size);
    renderMeterBar(size, problem);
    renderSizeLabel(size);
    renderSizeWarning(problem);
    primaryBtn.disabled = busy || empty || Boolean(problem);
  }

  function renderFileItem(file, index) {
    const li = document.createElement('li');
    li.className = 'file-item';
    const name = document.createElement('span');
    name.className = 'file-item-name';
    name.textContent = envelope.sanitizeFileName(file.name);
    const meta = document.createElement('span');
    meta.className = 'file-item-meta';
    meta.textContent = envelope.formatBytes(file.size);
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'file-item-remove';
    remove.textContent = '\u00d7';
    remove.setAttribute('aria-label', `Remove ${name.textContent}`);
    remove.addEventListener('click', function () { removeFile(index); });
    li.append(name, meta, remove);
    return li;
  }

  function renderFiles() {
    if (!fileList) return;
    fileList.replaceChildren();
    selectedFiles.forEach(function (f, i) { fileList.appendChild(renderFileItem(f, i)); });
    fileList.hidden = selectedFiles.length === 0;
    updateMeter();
  }

  function removeFile(index) {
    if (busy) return;
    selectedFiles.splice(index, 1);
    renderFiles();
    if (fileInput) fileInput.focus();
  }

  function isDuplicate(file) {
    return selectedFiles.some(function (f) {
      return f.name === file.name && f.size === file.size && f.lastModified === file.lastModified;
    });
  }

  function addFiles(list) {
    if (busy || !list) return;
    Array.from(list).forEach(function (f) {
      if (!isDuplicate(f)) selectedFiles.push(f);
    });
    if (fileInput) fileInput.value = '';
    clearError();
    renderFiles();
  }

  function setupDropZone() {
    const target = dropZone || form;
    ['dragenter', 'dragover'].forEach(function (type) {
      form.addEventListener(type, function (ev) {
        if (!ev.dataTransfer || !Array.from(ev.dataTransfer.types || []).includes('Files')) return;
        ev.preventDefault();
        target.classList.add('dragging');
      });
    });
    ['dragleave', 'dragend'].forEach(function (type) {
      form.addEventListener(type, function (ev) {
        if (ev.relatedTarget && form.contains(ev.relatedTarget)) return;
        target.classList.remove('dragging');
      });
    });
    form.addEventListener('drop', function (ev) {
      target.classList.remove('dragging');
      if (!ev.dataTransfer || !ev.dataTransfer.files.length) return;
      ev.preventDefault();
      addFiles(ev.dataTransfer.files);
    });
  }

  // --- Encrypt & upload ----------------------------------------------------
  async function buildPlaintext(message, files) {
    const buffers = await Promise.all(files.map(function (f) { return f.arrayBuffer(); }));
    const parts = files.map(function (f, i) {
      return { name: f.name, type: f.type, bytes: new Uint8Array(buffers[i]) };
    });
    const plaintext = envelope.encode(message, parts);
    parts.forEach(function (p) { p.bytes.fill(0); });
    return plaintext;
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

  function parseJSON(xhr) {
    try {
      return JSON.parse(xhr.responseText);
    } catch (_) {
      return null;
    }
  }

  // xhrUpload posts the ciphertext with XMLHttpRequest because fetch has no
  // upload progress events.
  function xhrUpload(ciphertext, headers) {
    return new Promise(function (resolve, reject) {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', '/api/secret');
      Object.keys(headers).forEach(function (k) { xhr.setRequestHeader(k, headers[k]); });
      xhr.upload.onprogress = function (e) {
        if (e.lengthComputable) setUploadProgress(e.loaded, e.total);
      };
      xhr.onload = function () { resolve({ status: xhr.status, json: parseJSON(xhr) }); };
      xhr.onerror = function () { reject(new Error('network error')); };
      xhr.onabort = function () { reject(new Error('upload aborted')); };
      xhr.ontimeout = function () { reject(new Error('upload timed out')); };
      xhr.send(ciphertext);
    });
  }

  function uploadErrorMessage(status) {
    if (status === 413) return 'Secret too large for this server';
    if (status === 429) return 'Slow down: too many requests. Please wait and retry.';
    if (status === 400) return 'The server rejected the secret';
    return 'Server error creating secret';
  }

  async function upload(encResult, ttl) {
    const headers = {
      'X-Gone-Version': String(window.goneCrypto.version),
      'X-Gone-Nonce': window.goneCrypto.b64urlEncode(encResult.nonce),
      'X-Gone-TTL': ttl,
      'Content-Type': 'application/octet-stream'
    };
    setUploadProgress(0, encResult.ciphertext.length);
    const t0 = performance.now();
    const res = await xhrUpload(encResult.ciphertext, headers);
    logTiming('upload', t0, performance.now());
    if (res.status !== 201 && res.status !== 200) {
      console.error('[gone] server error', res.status);
      throw new Error(uploadErrorMessage(res.status));
    }
    if (!res.json || !res.json.id) throw new Error('Unexpected server response');
    return res.json;
  }

  function buildShareURL(id, keyBytes) {
    const keyB64 = window.goneCrypto.exportKeyB64(keyBytes);
    return `${location.origin}/secret/${id}#v${window.goneCrypto.version}:${keyB64}`;
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

  async function encryptSelection(message, files) {
    setButtonLabel('Encrypting\u2026');
    const t0 = performance.now();
    const plaintext = await buildPlaintext(message, files);
    const keyBytes = window.goneCrypto.generateKey();
    try {
      const encResult = await window.goneCrypto.encrypt(plaintext, keyBytes);
      logTiming('encrypt', t0, performance.now());
      return { keyBytes: keyBytes, encResult: encResult };
    } finally {
      plaintext.fill(0);
    }
  }

  async function runSubmission() {
    const t0 = performance.now();
    const message = textarea.value;
    const ttl = ttlSelect.value;
    let enc;
    try {
      enc = await encryptSelection(message, selectedFiles.slice());
    } catch (e) {
      console.error('[gone] encryption failed', e);
      showError('Encryption failed: a file could not be read.');
      setBusy(false);
      return;
    }
    try {
      const json = await upload(enc.encResult, ttl);
      secureWipe(message);
      selectedFiles = [];
      logTiming('total_submit_cycle', t0, performance.now());
      buildAndShowResultPanel({ shareURL: buildShareURL(json.id, enc.keyBytes), expiresAt: json.expires_at, replaceTarget: cardSection });
    } catch (e) {
      console.error('[gone] upload failed', e);
      showError(e && e.message && !/network|abort|timed/.test(e.message) ? e.message : 'Network error uploading secret');
      setBusy(false);
    } finally {
      enc.keyBytes.fill(0);
    }
  }

  function handleSubmit(ev) {
    ev.preventDefault();
    if (busy) return;
    const problem = selectionProblem(currentSize());
    if (!textarea.value && selectedFiles.length === 0) {
      showError('Add a message or at least one file');
      return;
    }
    if (problem) {
      showError(problem);
      return;
    }
    clearError();
    setBusy(true);
    runSubmission();
  }

  form.addEventListener('submit', handleSubmit);
  textarea.addEventListener('input', updateMeter);
  if (fileInput) fileInput.addEventListener('change', function () { addFiles(fileInput.files); });
  setupDropZone();
  renderFiles();

  (function previewCheck() {
    const params = new URLSearchParams(location.search);
    if (params.get('preview') !== 'result') return;
    const mockID = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
    const mockKey = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';
    const mockURL = `${location.origin}/secret/${mockID}#v${window.goneCrypto.version}:${mockKey}`;
    const future = new Date(Date.now() + 30 * 60 * 1000).toISOString();
    buildAndShowResultPanel({ shareURL: mockURL, expiresAt: future, replaceTarget: cardSection, focus: false });
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
