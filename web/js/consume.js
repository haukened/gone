'use strict';

// Secret consumption (decrypt) flow extracted from app.js (refactored for lower complexity)
(function consumeFlow() {
  if (!window.goneCrypto) return;
  const container = document.getElementById('secret-consume');
  if (!container) return;

  // DOM references
  const statusEl = document.getElementById('secret-status');
  const outputTA = document.getElementById('secret-output');
  const actions = document.getElementById('secret-actions');
  const copyBtn = document.getElementById('copy-secret');

  function setStatus(msg){ if (statusEl) statusEl.textContent = msg; }
  function logTiming(label,start,end){ console.log(`[gone][timing] ${label}: ${(end-start).toFixed(2)}ms`); }

  // --- Helpers ------------------------------------------------------------
  function autoGrow(ta){ if (!ta) return; const max = 40*16; ta.style.height='auto'; const next=Math.min(ta.scrollHeight,max); ta.style.height=next+'px'; ta.style.overflowY=ta.scrollHeight>max?'auto':'hidden'; }

  function attachCopyHandler(text){
    if (!copyBtn) return;
    const COPY_ICON='<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-copy-icon lucide-copy"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';
    const CHECK_ICON='<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
    function revert(){ copyBtn.innerHTML='Copy Secret '+COPY_ICON; copyBtn.classList.remove('copied'); copyBtn.disabled=false; }
    copyBtn.addEventListener('click', async () => {
      let success=true; try { await navigator.clipboard.writeText(text); } catch(_){ success=false; }
      if (success){ copyBtn.innerHTML='Copied! '+CHECK_ICON; copyBtn.classList.add('copied'); copyBtn.disabled=true; setTimeout(revert,2200); }
      else { alert('Copy failed. Please press \u2318/Ctrl+C to copy manually.'); }
    });
  }

  function showPlaintext(text){
    if (outputTA){ outputTA.value=text; outputTA.hidden=false; autoGrow(outputTA); }
    if (actions) actions.hidden=false;
    setStatus('Decrypted');
    attachCopyHandler(text);
  }

  function handlePreview(params){
    const mockPlain=params.get('text')||'This is a preview of a decrypted secret. Customize via ?text=...';
    if (outputTA){ outputTA.value=mockPlain; outputTA.hidden=false; autoGrow(outputTA);} if(actions) actions.hidden=false; setStatus('Decrypted (preview)'); attachCopyHandler(mockPlain);
  }

  function parseFragment(hash){
    const m=/^#v(\d+):([A-Za-z0-9_-]{10,})$/.exec(hash||'');
    if(!m) return null;
    return { version: parseInt(m[1],10), keyB64:m[2] };
  }

  function validateIdFormat(id){ return /^[0-9a-f]{32}$/.test(id); }
  async function fetchSecret(id){
    // Defensive: ensure id is canonical 32-char lowercase hex before network use.
    if(!validateIdFormat(id)){ setStatus('Invalid secret id'); return null; }
    const safeId = encodeURIComponent(id); // path segment encoding (should be no-op for hex)
    setStatus('Fetching…');
    const t0=performance.now();
    const resp=await fetch(`/api/secret/${safeId}`);
    const t1=performance.now();
    logTiming('consume_fetch',t0,t1);
    if(!resp.ok){ setStatus(resp.status===404?'Secret not found or already consumed.':'Fetch error'); return null; }
    return resp;
  }

  function validateHeaders(resp){ const v=parseInt(resp.headers.get('X-Gone-Version')||'0',10); if(v!==window.goneCrypto.version){ setStatus('Version mismatch'); return null;} return resp.headers.get('X-Gone-Nonce')||''; }

  async function decryptPayload(resp,nonceB64,keyB64){ const nonce=window.goneCrypto.b64urlDecode(nonceB64); const ct=new Uint8Array(await resp.arrayBuffer()); const keyBytes=window.goneCrypto.importKeyB64(keyB64); const aad=new TextEncoder().encode('gone:v1'); const cryptoKey=await crypto.subtle.importKey('raw',keyBytes,{name:'AES-GCM'},false,['decrypt']); try { const t0=performance.now(); const pt=await crypto.subtle.decrypt({name:'AES-GCM',iv:nonce,additionalData:aad},cryptoKey,ct); const t1=performance.now(); logTiming('consume_decrypt',t0,t1); return new TextDecoder().decode(pt);} catch(_){ setStatus('Decryption failed'); return null; } }

  async function run(id,keyB64){
    try {
      const resp=await fetchSecret(id); if(!resp) return;
      const nonceB64=validateHeaders(resp); if(!nonceB64) return;
      const t0=performance.now();
      const plaintext=await decryptPayload(resp,nonceB64,keyB64); if(!plaintext) return;
      showPlaintext(plaintext);
      const t1=performance.now(); logTiming('consume_total',t0,t1);
    } catch(e){ console.error('[gone] consume error',e); setStatus('Unexpected error'); }
  }

  // --- Entry --------------------------------------------------------------
  const params=new URLSearchParams(location.search);
  if (params.get('preview')==='secret'){ handlePreview(params); return; }

  const frag=parseFragment(location.hash);
  if(!frag){ setStatus('Missing or invalid key fragment. Cannot decrypt.'); return; }
  if(frag.version!==window.goneCrypto.version){ setStatus('Unsupported version'); return; }
  const parts=location.pathname.split('/'); const id=parts[parts.length-1]; if(!id){ setStatus('Invalid secret id'); return; }
  run(id,frag.keyB64);
})();
