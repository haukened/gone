'use strict';

// Submission flow & result panel extracted from app.js
(function submitFlow() {
  const form = document.getElementById('create-secret');
  if (!form || !window.goneCrypto) return;
  const textarea = document.getElementById('secret');
  const ttlSelect = document.getElementById('ttl');
  const primaryBtn = form.querySelector('button[type="submit"]');
  const cardSection = form.closest('.card');
  if (!textarea || !ttlSelect || !primaryBtn || !cardSection) return;
  function logTiming(label, start, end) { console.log(`[gone][timing] ${label}: ${(end - start).toFixed(2)}ms`); }
  function setStatus(msg) { primaryBtn.textContent = msg; return true; }
  function secureWipe(raw) { try { const len = raw.length; textarea.value = ''.padEnd(len, '\u2022'); textarea.value = ''; } catch (_) { /* best-effort */ } }
  async function encryptSecret(raw) { const key = window.goneCrypto.generateKey(); const encStart = performance.now(); const encResult = await window.goneCrypto.encrypt(raw, key); const encEnd = performance.now(); logTiming('encrypt', encStart, encEnd); return { key, encResult }; }
  async function uploadCiphertext(encResult, keyBytes, ttl) { const { nonce, ciphertext } = encResult; const version = window.goneCrypto.version; const nonceB64 = window.goneCrypto.b64urlEncode(nonce); const uploadStart = performance.now(); setStatus('Uploading…'); const resp = await fetch('/api/secret', { method: 'POST', headers: { 'X-Gone-Version': String(version), 'X-Gone-Nonce': nonceB64, 'X-Gone-TTL': ttl, 'Content-Type': 'application/octet-stream' }, body: ciphertext }); const uploadEnd = performance.now(); logTiming('upload', uploadStart, uploadEnd); if (!resp.ok) { console.error('[gone] server error', resp.status); setStatus('Error'); return null; } return { json: await resp.json(), version, keyBytes }; }
  function buildShareURL(id, version, keyBytes) { const keyB64 = window.goneCrypto.exportKeyB64(keyBytes); return `${location.origin}/secret/${id}#v${version}:${keyB64}`; }
  function resetButton(original) { primaryBtn.disabled = false; primaryBtn.textContent = original; }
  function failureDelayReset(original, delay) { setTimeout(function () { resetButton(original); }, delay); }
  function logTotal(start) { const t1 = performance.now(); logTiming('total_submit_cycle', start, t1); }
  async function handleSubmit(ev) {
    ev.preventDefault(); function prepareSubmission() { const raw = textarea.value; if (!raw) { console.warn('[gone] empty secret submission blocked'); return null; } const ttl = ttlSelect.value; primaryBtn.disabled = true; return { raw, ttl, originalBtnHTML: primaryBtn.innerHTML, t0: performance.now() }; }
    async function performEncryption(raw, originalBtnHTML) { try { const { key, encResult } = await encryptSecret(raw); return { keyBytes: key, encResult }; } catch (e) { console.error('[gone] encryption failed', e); resetButton(originalBtnHTML); return null; } }
    async function performUpload(encResult, keyBytes, ttl, originalBtnHTML) { try { const res = await uploadCiphertext(encResult, keyBytes, ttl); if (!res) failureDelayReset(originalBtnHTML, 1200); return res; } catch (e) { console.error('[gone] upload failed', e); setStatus('Network Err'); failureDelayReset(originalBtnHTML, 1500); return null; } }
    function finalize(uploadRes, keyBytes, t0) { logTotal(t0); const shareURL = buildShareURL(uploadRes.json.ID, uploadRes.version, keyBytes); buildAndShowResultPanel({ shareURL, expiresAt: uploadRes.json.expires_at, replaceTarget: cardSection }); }
    const prep = prepareSubmission(); if (!prep) return; const { raw, ttl, originalBtnHTML, t0 } = prep; const encryption = await performEncryption(raw, originalBtnHTML); if (!encryption) return; secureWipe(raw); const uploadRes = await performUpload(encryption.encResult, encryption.keyBytes, ttl, originalBtnHTML); if (!uploadRes) return; finalize(uploadRes, encryption.keyBytes, t0);
  }
  form.addEventListener('submit', handleSubmit);
  (function previewCheck() { const params = new URLSearchParams(location.search); if (params.get('preview') === 'result') { const mockID = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'; const mockKey = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'; const version = window.goneCrypto.version; const mockURL = `${location.origin}/secret/${mockID}#v${version}:${mockKey}`; const future = new Date(Date.now() + 30 * 60 * 1000).toISOString(); buildAndShowResultPanel({ shareURL: mockURL, expiresAt: future, replaceTarget: form.closest('.card'), focus: false }); } })();
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
  if (focus && shareInput) shareInput.focus();
  if (copyBtnRoot) { copyBtnRoot.addEventListener('click', async function () { let success = true; try { await navigator.clipboard.writeText(shareURL); } catch (_) { if (shareInput) { shareInput.focus(); shareInput.select(); } success = false; } if (success) { copyBtnRoot.innerHTML = 'Copied! ' + CHECK_ICON; copyBtnRoot.classList.add('copied'); copyBtnRoot.disabled = true; setTimeout(function () { copyBtnRoot.innerHTML = 'Copy Link ' + COPY_ICON; copyBtnRoot.classList.remove('copied'); copyBtnRoot.disabled = false; }, 2200); } else { alert('Copy failed. Please press \u2318/Ctrl+C to copy manually.'); } return success; }); }
  console.log('[gone] result panel shown');
}
