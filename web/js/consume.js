'use strict';

// Secret consumption (decrypt) flow extracted from app.js
(function consumeFlow() {
  if (!window.goneCrypto) return;
  const container = document.getElementById('secret-consume');
  if (!container) return;
  const statusEl = document.getElementById('secret-status');
  const outputTA = document.getElementById('secret-output');
  const actions = document.getElementById('secret-actions');
  const copyBtn = document.getElementById('copy-secret');
  function setStatus(msg) { if (statusEl) statusEl.textContent = msg; }
  function logTiming(label, start, end) { console.log(`[gone][timing] ${label}: ${(end - start).toFixed(2)}ms`); }

  // Preview mode: /secret/anything#v1:MOCK?preview=secret OR /secret/mock#v1:MOCK&preview=secret
  // Allows iterating on presentation without creating a real secret.
  const urlParams = new URLSearchParams(location.search);
  const previewSecret = urlParams.get('preview') === 'secret';
  if (previewSecret) {
    const mockPlain = urlParams.get('text') || 'This is a preview of a decrypted secret. Customize via ?text=...';
    if (outputTA) {
      outputTA.value = mockPlain;
      outputTA.hidden = false;
      autoGrow(outputTA);
    }
    if (actions) actions.hidden = false;
    setStatus('Decrypted (preview)');
    attachCopyHandler(mockPlain);
    return; // Skip actual fetch/decrypt
  }
  const hash = location.hash || ''; const fragMatch = /^#v(\d+):([A-Za-z0-9_-]{10,})$/.exec(hash);
  if (!fragMatch) { setStatus('Missing or invalid key fragment. Cannot decrypt.'); return; }
  const fragVersion = parseInt(fragMatch[1], 10); const keyB64 = fragMatch[2];
  if (fragVersion !== window.goneCrypto.version) { setStatus('Unsupported version'); return; }
  const pathParts = location.pathname.split('/'); const id = pathParts[pathParts.length - 1]; if (!id) { setStatus('Invalid secret id'); return; }
  async function fetchSecret(secretID) { setStatus('Fetching…'); const tFetchStart = performance.now(); const resp = await fetch(`/api/secret/${secretID}`); const tFetchEnd = performance.now(); logTiming('consume_fetch', tFetchStart, tFetchEnd); if (!resp.ok) { setStatus(resp.status === 404 ? 'Secret not found or already consumed.' : 'Fetch error'); return null; } return resp; }
  function validateHeaders(resp) { const versionHdr = parseInt(resp.headers.get('X-Gone-Version') || '0', 10); if (versionHdr !== window.goneCrypto.version) { setStatus('Version mismatch'); return null; } const nonceB64 = resp.headers.get('X-Gone-Nonce') || ''; return nonceB64; }
  async function decryptPayload(resp, nonceB64, fragmentKeyB64) { const nonce = window.goneCrypto.b64urlDecode(nonceB64); const ctBuf = new Uint8Array(await resp.arrayBuffer()); const keyBytes = window.goneCrypto.importKeyB64(fragmentKeyB64); const additionalData = new TextEncoder().encode('gone:v1'); const cryptoKey = await crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, ['decrypt']); try { const tDecStart = performance.now(); const ptBuf = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce, additionalData }, cryptoKey, ctBuf); const tDecEnd = performance.now(); logTiming('consume_decrypt', tDecStart, tDecEnd); return new TextDecoder().decode(ptBuf); } catch (_) { setStatus('Decryption failed'); return null; } }
  function showPlaintext(text) {
    if (outputTA) {
      outputTA.value = text;
      outputTA.hidden = false;
      autoGrow(outputTA);
    }
    if (actions) actions.hidden = false;
    setStatus('Decrypted');
    attachCopyHandler(text);
  }
  function autoGrow(ta){
    if (!ta) return;
    const max = 40 * 16; // mirror create page constraint
    ta.style.height = 'auto';
    const next = Math.min(ta.scrollHeight, max);
    ta.style.height = next + 'px';
    ta.style.overflowY = ta.scrollHeight > max ? 'auto' : 'hidden';
  }
  function attachCopyHandler(text){
    if (!copyBtn) return;
    const COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-copy-icon lucide-copy"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';
    const CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
    function revert(){
      copyBtn.innerHTML = 'Copy Secret ' + COPY_ICON;
      copyBtn.classList.remove('copied');
      copyBtn.disabled = false;
    }
    copyBtn.addEventListener('click', async () => {
      let success = true;
      try { await navigator.clipboard.writeText(text); }
      catch (_) { success = false; }
      if (success) {
        copyBtn.innerHTML = 'Copied! ' + CHECK_ICON;
        copyBtn.classList.add('copied');
        copyBtn.disabled = true;
        setTimeout(revert, 2200);
      } else {
        alert('Copy failed. Please press \u2318/Ctrl+C to copy manually.');
      }
    }, { once: false });
  }
  (async function runConsume() { try { const resp = await fetchSecret(id); if (!resp) return; const nonceB64 = validateHeaders(resp); if (!nonceB64) return; const tStartTotal = performance.now(); const plaintext = await decryptPayload(resp, nonceB64, keyB64); if (!plaintext) return; showPlaintext(plaintext); const tEndTotal = performance.now(); logTiming('consume_total', tStartTotal, tEndTotal); } catch (e) { console.error('[gone] consume error', e); setStatus('Unexpected error'); } })();
})();
