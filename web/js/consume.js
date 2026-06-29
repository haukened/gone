'use strict';

// Secret consumption (decrypt) flow extracted from app.js (refactored for lower complexity)
(function consumeFlow() {
  if (!window.goneCrypto) return;
  const container = document.getElementById('secret-consume');
  if (!container) return;

  // DOM references
  // Status/heading element: template currently uses id "secret-heading".
  // Older code referenced an element id "secret-status" that no longer exists,
  // so runtime updates were not visible. We first try the current id and then
  // fall back for any cached/legacy template versions.
  const statusEl = document.getElementById('secret-heading') || document.getElementById('secret-status');
  const outputTA = document.getElementById('secret-output');
  const fileOutput = document.getElementById('file-output');
  const fileOutputName = document.getElementById('file-output-name');
  const fileOutputMeta = document.getElementById('file-output-meta');
  const copyBtn = document.getElementById('copy-secret');
  const downloadBtn = document.getElementById('download-file');
  const FILE_MAGIC = new TextEncoder().encode('GONEFILE1');
  let downloadURL = '';

  function setStatus(msg) {
    if (statusEl) statusEl.textContent = msg;
  }

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

  // --- Helpers ------------------------------------------------------------
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

  function hasFileMagic(bytes) {
    if (!bytes || bytes.length < FILE_MAGIC.length) return false;
    for (let i = 0; i < FILE_MAGIC.length; i++) {
      if (bytes[i] !== FILE_MAGIC[i]) return false;
    }
    return true;
  }

  function parseFileEnvelope(bytes) {
    if (!hasFileMagic(bytes)) return null;
    if (bytes.length < FILE_MAGIC.length + 4) throw new Error('truncated file envelope');
    let offset = FILE_MAGIC.length;
    const metadataLength = (
      (bytes[offset] << 24) |
      (bytes[offset + 1] << 16) |
      (bytes[offset + 2] << 8) |
      bytes[offset + 3]
    ) >>> 0;
    offset += 4;
    if (metadataLength > bytes.length - offset) throw new Error('invalid file metadata length');
    const metadataBytes = bytes.subarray(offset, offset + metadataLength);
    offset += metadataLength;
    const metadata = JSON.parse(new TextDecoder().decode(metadataBytes));
    const content = bytes.subarray(offset);
    if (!metadata || metadata.kind !== 'file') throw new Error('invalid file metadata');
    const name = sanitizeFileName(metadata.name);
    const type = typeof metadata.type === 'string' && metadata.type ? metadata.type : 'application/octet-stream';
    const size = Number(metadata.size);
    if (!Number.isFinite(size) || size < 0 || size !== content.length) throw new Error('file size mismatch');
    return { name, type, size, content };
  }

  function autoGrow(ta) {
    if (!ta) return;
    const max = 40 * 16;
    ta.style.height = 'auto';
    const next = Math.min(ta.scrollHeight, max);
    ta.style.height = next + 'px';
    ta.style.overflowY = ta.scrollHeight > max ? 'auto' : 'hidden';
  }

  function attachCopyHandler(text) {
    if (!copyBtn) return;
    const COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-copy-icon lucide-copy"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';
    const CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
    function revert() {
      copyBtn.innerHTML = 'Copy Secret ' + COPY_ICON;
      copyBtn.classList.remove('copied');
      copyBtn.disabled = false;
    }
    copyBtn.addEventListener('click', async () => {
      let success = true;
      try {
        await navigator.clipboard.writeText(text);
      } catch (_) {
        success = false;
      }
      if (success) {
        copyBtn.innerHTML = 'Copied! ' + CHECK_ICON;
        copyBtn.classList.add('copied');
        copyBtn.disabled = true;
        setTimeout(revert, 2200);
      } else {
        alert('Copy failed. Please press \u2318/Ctrl+C to copy manually.');
      }
    });
  }

  function showPlaintext(text) {
    if (outputTA) {
      outputTA.value = text;
      outputTA.hidden = false;
      autoGrow(outputTA);
    }
    if (fileOutput) fileOutput.hidden = true;
    if (downloadBtn) downloadBtn.hidden = true;
    // Reveal copy button now that plaintext is available
    if (copyBtn) copyBtn.hidden = false;
    setStatus('Decrypted Secret:');
    attachCopyHandler(text);
  }

  function showFile(filePayload) {
    if (outputTA) {
      outputTA.value = '';
      outputTA.hidden = true;
    }
    if (copyBtn) copyBtn.hidden = true;
    if (fileOutputName) fileOutputName.textContent = filePayload.name;
    if (fileOutputMeta) fileOutputMeta.textContent = `${formatBytes(filePayload.size)} - ${filePayload.type}`;
    if (fileOutput) fileOutput.hidden = false;
    if (downloadURL) URL.revokeObjectURL(downloadURL);
    const blob = new Blob([filePayload.content], { type: filePayload.type });
    downloadURL = URL.createObjectURL(blob);
    if (downloadBtn) {
      downloadBtn.hidden = false;
      downloadBtn.onclick = function () {
        const link = document.createElement('a');
        link.href = downloadURL;
        link.download = filePayload.name;
        document.body.appendChild(link);
        link.click();
        link.remove();
      };
    }
    setStatus('Decrypted File:');
  }

  function handlePreview(params) {
    const mockPlain = params.get('text') || 'This is a preview of a decrypted secret. Customize via ?text=...';
    if (outputTA) {
      outputTA.value = mockPlain;
      outputTA.hidden = false;
      autoGrow(outputTA);
    }
    if (fileOutput) fileOutput.hidden = true;
    if (downloadBtn) downloadBtn.hidden = true;
    // Reveal copy button for preview mode
    if (copyBtn) copyBtn.hidden = false;
    setStatus('Decrypted (preview)');
    attachCopyHandler(mockPlain);
  }

  function parseFragment(hash) {
    const m = /^#v(\d+):([A-Za-z0-9_-]{10,})$/.exec(hash || '');
    if (!m) return null;
    return { version: parseInt(m[1], 10), keyB64: m[2] };
  }

  function validateIdFormat(id) {
    return /^[0-9a-f]{32}$/.test(id);
  }

  async function fetchSecret(id) {
    if (!validateIdFormat(id)) {
      setStatus('Invalid secret id');
      return null;
    }
    const safeId = encodeURIComponent(id);
    setStatus('Fetching…');
    const t0 = performance.now();
    const resp = await fetch(`/api/secret/${safeId}`);
    const t1 = performance.now();
    logTiming('consume_fetch', t0, t1);
    if (!resp.ok) {
      if (resp.status === 404 || resp.status === 410) {
        setStatus('That secret either never existed, has already been consumed, or has expired.');
      } else if (resp.status === 429) {
        setStatus('Rate limited. Please wait and retry.');
      } else {
        setStatus('Fetch error');
      }
      return null;
    }
    return resp;
  }

  function validateHeaders(resp) {
    const v = parseInt(resp.headers.get('X-Gone-Version') || '0', 10);
    if (v !== window.goneCrypto.version) {
      setStatus('Version mismatch');
      return null;
    }
    return resp.headers.get('X-Gone-Nonce') || '';
  }

  async function decryptPayload(resp, nonceB64, keyB64) {
    const nonce = window.goneCrypto.b64urlDecode(nonceB64);
    const ct = new Uint8Array(await resp.arrayBuffer());
    const keyBytes = window.goneCrypto.importKeyB64(keyB64);
    const aad = new TextEncoder().encode('gone:v1');
    const cryptoKey = await crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, ['decrypt']);
    try {
      const t0 = performance.now();
      const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce, additionalData: aad }, cryptoKey, ct);
      const t1 = performance.now();
      logTiming('consume_decrypt', t0, t1);
      return new Uint8Array(pt);
    } catch (_) {
      setStatus('Decryption failed');
      return null;
    }
  }

  async function run(id, keyB64) {
    try {
      const resp = await fetchSecret(id);
      if (!resp) return;
      const nonceB64 = validateHeaders(resp);
      if (!nonceB64) return;
      const t0 = performance.now();
      const plaintextBytes = await decryptPayload(resp, nonceB64, keyB64);
      if (plaintextBytes === null) return;
      let filePayload;
      try {
        filePayload = parseFileEnvelope(plaintextBytes);
      } catch (e) {
        console.error('[gone] invalid file envelope', e);
        setStatus('Invalid file payload');
        return;
      }
      if (filePayload) {
        showFile(filePayload);
      } else {
        showPlaintext(new TextDecoder().decode(plaintextBytes));
      }
      const t1 = performance.now();
      logTiming('consume_total', t0, t1);
    } catch (e) {
      console.error('[gone] consume error', e);
      setStatus('Unexpected error');
    }
  }

  // --- Entry --------------------------------------------------------------
  const params = new URLSearchParams(location.search);
  if (params.get('preview') === 'secret') {
    handlePreview(params);
    return;
  }

  const frag = parseFragment(location.hash);
  if (!frag) {
    setStatus('Missing or invalid key fragment. Cannot decrypt.');
    return;
  }
  if (frag.version !== window.goneCrypto.version) {
    setStatus('Unsupported version');
    return;
  }
  const parts = location.pathname.split('/');
  const id = parts[parts.length - 1];
  if (!id) {
    setStatus('Invalid secret id');
    return;
  }
  window.addEventListener('beforeunload', function () {
    if (downloadURL) URL.revokeObjectURL(downloadURL);
  });
  run(id, frag.keyB64);
})();
