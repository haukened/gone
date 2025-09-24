'use strict';

// Secret consumption (decrypt) flow extracted from app.js
(function consumeFlow() {
  if (!window.goneCrypto) return;
  const container = document.getElementById('secret-consume');
  if (!container) return;
  const statusEl = document.getElementById('secret-status');
  const pre = document.getElementById('secret-plaintext');
  const actions = document.getElementById('secret-actions');
  const copyBtn = document.getElementById('copy-secret');
  function setStatus(msg) { if (statusEl) statusEl.textContent = msg; }
  function logTiming(label, start, end) { console.log(`[gone][timing] ${label}: ${(end - start).toFixed(2)}ms`); }
  const hash = location.hash || ''; const fragMatch = /^#v(\d+):([A-Za-z0-9_-]{10,})$/.exec(hash);
  if (!fragMatch) { setStatus('Missing or invalid key fragment. Cannot decrypt.'); return; }
  const fragVersion = parseInt(fragMatch[1], 10); const keyB64 = fragMatch[2];
  if (fragVersion !== window.goneCrypto.version) { setStatus('Unsupported version'); return; }
  const pathParts = location.pathname.split('/'); const id = pathParts[pathParts.length - 1]; if (!id) { setStatus('Invalid secret id'); return; }
  async function fetchSecret(secretID) { setStatus('Fetching…'); const tFetchStart = performance.now(); const resp = await fetch(`/api/secret/${secretID}`); const tFetchEnd = performance.now(); logTiming('consume_fetch', tFetchStart, tFetchEnd); if (!resp.ok) { setStatus(resp.status === 404 ? 'Secret not found or already consumed.' : 'Fetch error'); return null; } return resp; }
  function validateHeaders(resp) { const versionHdr = parseInt(resp.headers.get('X-Gone-Version') || '0', 10); if (versionHdr !== window.goneCrypto.version) { setStatus('Version mismatch'); return null; } const nonceB64 = resp.headers.get('X-Gone-Nonce') || ''; return nonceB64; }
  async function decryptPayload(resp, nonceB64, fragmentKeyB64) { const nonce = window.goneCrypto.b64urlDecode(nonceB64); const ctBuf = new Uint8Array(await resp.arrayBuffer()); const keyBytes = window.goneCrypto.importKeyB64(fragmentKeyB64); const additionalData = new TextEncoder().encode('gone:v1'); const cryptoKey = await crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, ['decrypt']); try { const tDecStart = performance.now(); const ptBuf = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce, additionalData }, cryptoKey, ctBuf); const tDecEnd = performance.now(); logTiming('consume_decrypt', tDecStart, tDecEnd); return new TextDecoder().decode(ptBuf); } catch (_) { setStatus('Decryption failed'); return null; } }
  function showPlaintext(text) { if (pre) { pre.textContent = text; pre.hidden = false; } if (actions) actions.hidden = false; setStatus('Decrypted'); if (copyBtn) { copyBtn.addEventListener('click', async () => { try { await navigator.clipboard.writeText(text); copyBtn.textContent = 'Copied!'; setTimeout(() => copyBtn.textContent = 'Copy Secret', 2500); } catch (_) { copyBtn.textContent = 'Copy failed'; setTimeout(() => copyBtn.textContent = 'Copy Secret', 2500); } }); } }
  (async function runConsume() { try { const resp = await fetchSecret(id); if (!resp) return; const nonceB64 = validateHeaders(resp); if (!nonceB64) return; const tStartTotal = performance.now(); const plaintext = await decryptPayload(resp, nonceB64, keyB64); if (!plaintext) return; showPlaintext(plaintext); const tEndTotal = performance.now(); logTiming('consume_total', tStartTotal, tEndTotal); } catch (e) { console.error('[gone] consume error', e); setStatus('Unexpected error'); } })();
})();
