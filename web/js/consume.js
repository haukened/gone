'use strict';

// Secret consumption flow: claim (streamed GET) -> decrypt -> decode -> ack (DELETE).
// The secret is only deleted from the server once the full payload has been
// received and authenticated by AES-GCM, so interrupted downloads can retry.
(function consumeFlow() {
  if (!window.goneCrypto || !window.goneEnvelope) return;
  const container = document.getElementById('secret-consume');
  if (!container) return;
  const envelope = window.goneEnvelope;

  const statusEl = document.getElementById('secret-heading');
  const outputTA = document.getElementById('secret-output');
  const copyBtn = document.getElementById('copy-secret');
  const downloadAllBtn = document.getElementById('download-all');
  const progressEl = document.getElementById('download-progress');
  const fileSection = document.getElementById('file-section');
  const fileListEl = document.getElementById('file-output-list');
  const ackBanner = document.getElementById('ack-banner');
  const ackCard = document.getElementById('ack-card');
  const ackTitle = document.getElementById('ack-title');
  const ackText = document.getElementById('ack-text');

  const MAX_FETCH_ATTEMPTS = 3;
  const MAX_ACK_ATTEMPTS = 3;
  const API_PATH = '/api/secret/';
  const SECRET_ID_RE = /^[0-9a-f]{32}$/;
  const state = { plaintext: null, files: [], urls: [], pending: 0 };

  function setStatus(msg) {
    if (statusEl) statusEl.textContent = msg;
  }

  const debugTiming = (function () {
    try {
      const params = new URLSearchParams(location.search);
      if (params.get('debug') === 'timing') return true;
      return (window.localStorage && localStorage.getItem('goneDebugTiming') === '1');
    } catch (_) { return false; }
  })();

  function logTiming(label, begin, end) {
    if (!debugTiming) return;
    console.log(`[gone][timing] ${label}: ${(end - begin).toFixed(2)}ms`);
  }

  function sleep(ms) {
    return new Promise(function (resolve) { setTimeout(resolve, ms); });
  }

  // FetchError carries a user-facing message and whether retrying may help.
  function FetchError(message, retryable) {
    const e = new Error(message);
    e.retryable = retryable;
    return e;
  }

  // --- Retrieval -------------------------------------------------------------
  function statusMessage(status) {
    if (status === 404 || status === 410) {
      return 'This secret is gone: it never existed, was already opened, has expired, or was opened elsewhere. If your own download was interrupted, ask the sender to share it again.';
    }
    if (status === 429) return 'Slow down: too many requests. Please wait and retry.';
    if (status === 400) return 'Invalid secret link';
    return 'Server error retrieving secret';
  }

  function setProgress(received, total) {
    if (!progressEl) return;
    progressEl.hidden = false;
    if (total > 0) {
      progressEl.max = total;
      progressEl.value = Math.min(received, total);
      setStatus(`Retrieving\u2026 ${Math.floor((received / total) * 100)}%`);
    } else {
      progressEl.removeAttribute('value');
      setStatus(`Retrieving\u2026 ${envelope.formatBytes(received)}`);
    }
  }

  function concatChunks(chunks, length) {
    const out = new Uint8Array(length);
    let offset = 0;
    chunks.forEach(function (c) { out.set(c, offset); offset += c.length; c.fill(0); });
    return out;
  }

  async function readBody(resp, total) {
    if (!resp.body || !resp.body.getReader) {
      return new Uint8Array(await resp.arrayBuffer());
    }
    const reader = resp.body.getReader();
    const chunks = [];
    let received = 0;
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
      received += value.length;
      setProgress(received, total);
    }
    return concatChunks(chunks, received);
  }

  // secretEndpoint is the only place an API URL is built. It accepts nothing but
  // a 32-char hex id and pins the result to this page's origin, so no
  // user-controlled value can redirect requests elsewhere.
  function secretEndpoint(id) {
    if (typeof id !== 'string' || !SECRET_ID_RE.test(id)) throw FetchError('Invalid secret id', false);
    const origin = window.location.origin;
    const endpoint = new URL(API_PATH + id, origin);
    if (endpoint.origin !== origin || endpoint.pathname !== API_PATH + id) {
      throw FetchError('Invalid secret id', false);
    }
    return endpoint.href;
  }

  // apiInit builds fetch options restricted to same-origin, no redirects, no cache.
  function apiInit(method, headers) {
    return {
      method: method,
      headers: headers,
      mode: 'same-origin',
      credentials: 'same-origin',
      redirect: 'error',
      cache: 'no-store',
      keepalive: method === 'DELETE'
    };
  }

  // requestSecret issues the GET (presenting any claim token) and records the
  // claim token the server returns.
  async function requestSecret(claim) {
    const endpoint = secretEndpoint(claim.id);
    const headers = claim.token ? { 'X-Gone-Claim': claim.token } : {};
    let resp;
    try {
      resp = await fetch(endpoint, apiInit('GET', headers));
    } catch (_) {
      throw FetchError('Network error retrieving secret', true);
    }
    if (!resp.ok) throw FetchError(statusMessage(resp.status), resp.status >= 500);
    claim.token = resp.headers.get('X-Gone-Claim') || claim.token;
    return resp;
  }

  // readComplete reads the whole body and rejects a truncated download.
  async function readComplete(resp) {
    const total = parseInt(resp.headers.get('Content-Length') || '0', 10) || 0;
    let body;
    try {
      body = await readBody(resp, total);
    } catch (_) {
      throw FetchError('Network error retrieving secret', true);
    }
    if (total && body.length !== total) throw FetchError('Download was incomplete', true);
    return body;
  }

  async function fetchOnce(claim) {
    const resp = await requestSecret(claim);
    const body = await readComplete(resp);
    return { resp: resp, body: body };
  }

  // fetchWithRetry retries network failures/truncation, presenting the claim
  // token so the server re-serves the same claim instead of reporting it gone.
  async function fetchWithRetry(claim) {
    let lastErr;
    for (let attempt = 1; attempt <= MAX_FETCH_ATTEMPTS; attempt++) {
      try {
        return await fetchOnce(claim);
      } catch (e) {
        lastErr = e;
        if (!e.retryable || (attempt > 1 && !claim.token)) break;
        console.warn('[gone] retrieval attempt failed, retrying', attempt);
        await sleep(500 * attempt);
      }
    }
    throw lastErr;
  }

  // --- Crypto ------------------------------------------------------------------
  async function decrypt(resp, ciphertext, keyB64) {
    const version = parseInt(resp.headers.get('X-Gone-Version') || '0', 10);
    if (version !== window.goneCrypto.version) throw FetchError('Unsupported secret version', false);
    const nonce = window.goneCrypto.b64urlDecode(resp.headers.get('X-Gone-Nonce') || '');
    const keyBytes = window.goneCrypto.importKeyB64(keyB64);
    const aad = new TextEncoder().encode('gone:v1');
    try {
      const key = await crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, ['decrypt']);
      const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce, additionalData: aad }, key, ciphertext);
      return new Uint8Array(pt);
    } catch (_) {
      throw FetchError('Couldn\u2019t verify this secret. The link may be incomplete or wrong; ask the sender to resend it.', false);
    } finally {
      keyBytes.fill(0);
      ciphertext.fill(0);
    }
  }

  // --- Acknowledge (delete) ---------------------------------------------------
  async function ackOnce(claim) {
    const endpoint = secretEndpoint(claim.id);
    const resp = await fetch(endpoint, apiInit('DELETE', { 'X-Gone-Claim': claim.token }));
    return resp.status === 204;
  }

  async function acknowledge(claim) {
    for (let attempt = 1; attempt <= MAX_ACK_ATTEMPTS; attempt++) {
      try {
        if (await ackOnce(claim)) return true;
      } catch (_) {
        // retry
      }
      await sleep(500 * attempt);
    }
    return false;
  }

  function showAckResult(ok) {
    if (!ackBanner) return;
    if (!ok) {
      if (ackCard) ackCard.classList.add('danger');
      if (ackTitle) ackTitle.textContent = 'Couldn\u2019t confirm deletion';
      if (ackText) ackText.textContent = 'The server did not confirm this secret was deleted. It will still be deleted automatically within a few minutes and cannot be opened again. Save what you need now.';
    }
    ackBanner.hidden = false;
  }

  // --- Rendering ----------------------------------------------------------------
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
      try {
        await navigator.clipboard.writeText(text);
      } catch (_) {
        alert('Copy failed. Please press \u2318/Ctrl+C to copy manually.');
        return;
      }
      copyBtn.innerHTML = 'Copied! ' + CHECK_ICON;
      copyBtn.classList.add('copied');
      copyBtn.disabled = true;
      setTimeout(revert, 2200);
    });
  }

  function showMessage(text) {
    if (!text || !outputTA) return;
    outputTA.value = text;
    outputTA.hidden = false;
    autoGrow(outputTA);
    if (copyBtn) copyBtn.hidden = false;
    attachCopyHandler(text);
  }

  function fileURL(entry) {
    if (!entry.url) {
      entry.url = URL.createObjectURL(new Blob([entry.file.bytes], { type: entry.file.type }));
      state.urls.push(entry.url);
    }
    return entry.url;
  }

  function downloadEntry(entry) {
    const link = document.createElement('a');
    link.href = fileURL(entry);
    link.download = entry.file.name;
    link.rel = 'noopener';
    document.body.appendChild(link);
    link.click();
    link.remove();
    if (!entry.done) {
      entry.done = true;
      state.pending--;
      entry.li.classList.add('downloaded');
    }
  }

  function renderFileEntry(file) {
    const entry = { file: file, url: '', done: false, li: document.createElement('li') };
    entry.li.className = 'file-item';
    const name = document.createElement('span');
    name.className = 'file-item-name';
    name.textContent = file.name;
    const meta = document.createElement('span');
    meta.className = 'file-item-meta';
    meta.textContent = envelope.formatBytes(file.size);
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'secondary-btn';
    btn.textContent = 'Download';
    btn.setAttribute('aria-label', `Download ${file.name}`);
    btn.addEventListener('click', function () { downloadEntry(entry); });
    entry.li.append(name, meta, btn);
    return entry;
  }

  async function downloadAll() {
    for (const entry of state.files) {
      downloadEntry(entry);
      // Space out downloads so browsers don't drop or batch-block them.
      await sleep(400);
    }
  }

  function showFiles(files) {
    if (!files.length || !fileListEl) return;
    state.files = files.map(renderFileEntry);
    state.pending = state.files.length;
    state.files.forEach(function (e) { fileListEl.appendChild(e.li); });
    if (fileSection) fileSection.hidden = false;
    if (downloadAllBtn) {
      downloadAllBtn.hidden = files.length < 2;
      downloadAllBtn.addEventListener('click', downloadAll);
    }
  }

  function headingFor(decoded) {
    const n = decoded.files.length;
    const files = n === 1 ? '1 file' : `${n} files`;
    if (decoded.message && n) return `Decrypted Secret + ${files}:`;
    if (n) return `Decrypted ${files}:`;
    return 'Decrypted Secret:';
  }

  // --- Lifecycle -----------------------------------------------------------------
  function guardUnload(ev) {
    if (state.pending <= 0) return;
    ev.preventDefault();
    ev.returnValue = '';
  }

  function cleanup() {
    state.urls.forEach(function (u) { URL.revokeObjectURL(u); });
    state.urls = [];
    if (state.plaintext) state.plaintext.fill(0);
  }

  async function run(id, keyB64) {
    const t0 = performance.now();
    const claim = { id: id, token: '' };
    setStatus('Retrieving\u2026');
    const fetched = await fetchWithRetry(claim);
    logTiming('consume_fetch', t0, performance.now());
    setStatus('Decrypting\u2026');
    state.plaintext = await decrypt(fetched.resp, fetched.body, keyB64);
    let decoded;
    try {
      decoded = envelope.decode(state.plaintext);
    } catch (e) {
      console.error('[gone] invalid envelope', e);
      throw FetchError('This secret\u2019s contents are malformed; ask the sender to resend it.', false);
    }
    if (progressEl) progressEl.hidden = true;
    setStatus(headingFor(decoded));
    showMessage(decoded.message);
    showFiles(decoded.files);
    logTiming('consume_total', t0, performance.now());
    showAckResult(await acknowledge(claim));
  }

  function handlePreview(params) {
    const text = params.get('text') || 'This is a preview of a decrypted secret. Customize via ?text=...';
    showMessage(text);
    setStatus('Decrypted (preview)');
  }

  function parseFragment(hash) {
    const m = /^#v(\d+):([A-Za-z0-9_-]{10,})$/.exec(hash || '');
    if (!m) return null;
    return { version: parseInt(m[1], 10), keyB64: m[2] };
  }

  function start() {
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
    if (!SECRET_ID_RE.test(id)) {
      setStatus('Invalid secret id');
      return;
    }
    window.addEventListener('beforeunload', guardUnload);
    window.addEventListener('pagehide', cleanup);
    run(id, frag.keyB64).catch(function (e) {
      console.error('[gone] consume error', e && e.retryable !== undefined ? e.message : e);
      if (progressEl) progressEl.hidden = true;
      setStatus(e && e.retryable !== undefined ? e.message : 'Unexpected error');
    });
  }

  start();
})();
