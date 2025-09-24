/* eslint-env browser */
/* eslint-disable no-restricted-globals */
'use strict';

// Theme persistence & future extension hook.
// Responsibilities:
// 1. On load, determine desired theme: stored value OR system preference.
// 2. Apply by setting the checkbox state (which drives CSS via :has selector).
// 3. Persist user toggles to localStorage.
// 4. Maintain accessible state attributes (aria-pressed + description) to reflect current/next theme.

(function() {
  const STORAGE_KEY = 'gone.theme'; // values: 'light' | 'dark'
  const checkbox = document.getElementById('theme-switch');
  if (!checkbox) {
    return false;
  }

  // guard against server-side rendering (e.g. static export)
  if (typeof window === 'undefined') { return false; }

  const desc = document.getElementById('theme-switch-desc');

  function systemPrefersDark() {
    try {
      if (typeof window !== 'undefined' && window.matchMedia) {
        return !!window.matchMedia('(prefers-color-scheme: dark)').matches;
      }
    } catch(_) { return false; }
    return false;
  }

  function applyTheme(mode, persistParam) {
    const persist = (persistParam === undefined) ? true : persistParam;
    const isDark = mode === 'dark';
    checkbox.checked = isDark; // checked => dark variables active (CSS :has)
    checkbox.setAttribute('aria-pressed', String(isDark));
    if (desc) {
      desc.textContent = isDark
        ? 'Toggle theme. Currently dark; activates light mode.'
        : 'Toggle theme. Currently light; activates dark mode.';
    }
    if (persist) {
      try { localStorage.setItem(STORAGE_KEY, mode); } catch (_) { return false; }
    }
    return true;
  }

  function loadInitialTheme() {
    let stored = "";
    try {
      stored = localStorage.getItem(STORAGE_KEY);
    } catch (_) { return false; }

    if (stored !== 'light' && stored !== 'dark') {
      stored = systemPrefersDark() ? 'dark' : 'light';
      try { localStorage.setItem(STORAGE_KEY, stored); } catch (_) { return false; }
    }
    applyTheme(stored, false);
    return true;
  }

  checkbox.addEventListener('change', function(){
    const mode = checkbox.checked ? 'dark' : 'light';
    applyTheme(mode, true);
    return true;
  });

  // React to system preference changes if user has not explicitly overridden (only when no stored value).
  if (!localStorage.getItem(STORAGE_KEY) && window.matchMedia) {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    mq.addEventListener('change', function(e){
      if (!localStorage.getItem(STORAGE_KEY)) {
        applyTheme(e.matches ? 'dark' : 'light', true);
      }
      return true;
      });
  }

  loadInitialTheme();

  // Security warning: show if served over insecure HTTP (excluding localhost & 127.0.0.1 & ::1)
  (function securityWarning(){
    try {
      const insecure = window.location.protocol === 'http:';
      if (insecure) {
        const section = document.querySelector('.security-warning');
        if (section) {
          section.hidden = false;
          section.setAttribute('aria-hidden', 'false');
        }
      }
    } catch(_) { return false; }
    return true;
  })();

  // Placeholder for future client-side encryption workflow bootstrapping.
  console.log('Gone UI loaded');
  return true;
})();

(function autoResize() {
  const ta = document.getElementById('secret');
  if (!ta) return false;
  const max = 40 * 16; // 40rem assuming 16px root, adjust as needed
  function grow() {
    ta.style.height = 'auto';
    const next = Math.min(ta.scrollHeight, max);
    ta.style.height = next + 'px';
    ta.style.overflowY = ta.scrollHeight > max ? 'auto' : 'hidden';
    return true;
  }
  ta.addEventListener('input', grow);
  grow();
  return true;
})();

// AES-GCM v1 client-side crypto scaffold.
// Transport format currently: raw ciphertext bytes sent as body + nonce provided in X-Gone-Nonce header.
// Key (32 random bytes) is never sent to server; represented in URL fragment as: v1:<base64url(key)>.
// Public API attached at window.goneCrypto { generateKey(), encrypt(plaintext|Uint8Array) } (decrypt helper removed – consumption path performs direct subtle.decrypt with nonce + AAD).
(function cryptoScaffold(){
  const VERSION = 0x01; // increment for future formats
  const KEY_BYTES = 32; // 256-bit AES-GCM
  const NONCE_BYTES = 12; // 96-bit nonce per NIST recommendation

  if (window.goneCrypto) {
    return; // already defined
  }

  // Utilities
  function utf8Encode(str) { return new TextEncoder().encode(str); }
  function b64urlEncode(bytes) {
    let bin = '';
    for (let i=0;i<bytes.length;i++) bin += String.fromCharCode(bytes[i]);
    const b64 = btoa(bin).replace(/\+/g,'-').replace(/\//g,'_');
    let end = b64.length; // manual trim '=' padding instead of regex quantifier
    while (end > 0 && b64.charAt(end-1) === '=') end--;
    return b64.substring(0, end);
  }
  function b64urlDecode(s) {
    let norm = s.replace(/-/g,'+').replace(/_/g,'/');
    while (norm.length % 4) norm += '=';
    const bin = atob(norm);
    const out = new Uint8Array(bin.length);
    for (let i=0;i<bin.length;i++) out[i] = bin.charCodeAt(i);
    return out;
  }
  function randomBytes(n) {
    const b = new Uint8Array(n);
    crypto.getRandomValues(b);
    return b;
  }

  async function importKey(raw) {
    return crypto.subtle.importKey('raw', raw, { name: 'AES-GCM' }, false, ['encrypt','decrypt']);
  }

  function generateKey() {
    return randomBytes(KEY_BYTES);
  }

  async function encrypt(data, keyBytes) {
    if (!(keyBytes instanceof Uint8Array) || keyBytes.length !== KEY_BYTES) {
      throw new Error('invalid key length');
    }
    let plaintextBytes;
    if (typeof data === 'string') {
      plaintextBytes = utf8Encode(data);
    } else if (data instanceof Uint8Array) {
      plaintextBytes = data;
    } else {
      throw new Error('data must be string or Uint8Array');
    }
    const key = await importKey(keyBytes);
    const nonce = randomBytes(NONCE_BYTES);
    const additionalData = new TextEncoder().encode('gone:v1');
    const ctBuf = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce, additionalData }, key, plaintextBytes);
    const ct = new Uint8Array(ctBuf);
      return { nonce, ciphertext: ct };
  }

  function exportKeyB64(keyBytes) { return b64urlEncode(keyBytes); }
  function importKeyB64(k) { const b = b64urlDecode(k); if (b.length !== KEY_BYTES) throw new Error('invalid key'); return b; }

  window.goneCrypto = {
    version: VERSION,
    generateKey,
    exportKeyB64,
    importKeyB64,
    encrypt,
    b64urlEncode,
    b64urlDecode
  };
})();

  // Form submission handling with timing instrumentation and result panel.
  (function submitFlow(){
    const form = document.getElementById('create-secret');
    if (!form || !window.goneCrypto) return;
    const textarea = document.getElementById('secret');
    const ttlSelect = document.getElementById('ttl');
    const primaryBtn = form.querySelector('button[type="submit"]');
    const cardSection = form.closest('.card');
    if (!textarea || !ttlSelect || !primaryBtn || !cardSection) return;

    function logTiming(label, start, end){
      const ms = (end - start).toFixed(2);
      console.log(`[gone][timing] ${label}: ${ms}ms`);
    }

    function setStatus(msg){ primaryBtn.textContent = msg; return true; }

    function secureWipe(raw){
      try {
        const len = raw.length;
        textarea.value = ''.padEnd(len, '\u2022');
        textarea.value = '';
      } catch(_) {
        // Intentionally ignore wipe failures (e.g., DOM not writable); best-effort.
      }
    }

    async function encryptSecret(raw){
      const key = window.goneCrypto.generateKey();
      const encStart = performance.now();
      const encResult = await window.goneCrypto.encrypt(raw, key);
      const encEnd = performance.now();
      logTiming('encrypt', encStart, encEnd);
      return { key, encResult };
    }

    async function uploadCiphertext(encResult, keyBytes, ttl){
      const { nonce, ciphertext } = encResult;
      const version = window.goneCrypto.version;
      const nonceB64 = window.goneCrypto.b64urlEncode(nonce);
      const uploadStart = performance.now();
      setStatus('Uploading…');
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
        setStatus('Error');
        return null;
      }
      return { json: await resp.json(), version, keyBytes };
    }

    function buildShareURL(id, version, keyBytes){
      const keyB64 = window.goneCrypto.exportKeyB64(keyBytes);
      return `${location.origin}/secret/${id}#v${version}:${keyB64}`;
    }

    function resetButton(original){
      primaryBtn.disabled = false;
      primaryBtn.textContent = original;
    }

    function failureDelayReset(original, delay){
      setTimeout(function(){ resetButton(original); }, delay);
    }

    function logTotal(start){
      const t1 = performance.now();
      logTiming('total_submit_cycle', start, t1);
    }

    async function handleSubmit(ev){
      ev.preventDefault();

      function prepareSubmission(){
        const raw = textarea.value;
        if (!raw) { console.warn('[gone] empty secret submission blocked'); return null; }
        const ttl = ttlSelect.value;
        primaryBtn.disabled = true;
        return { raw, ttl, originalBtnHTML: primaryBtn.innerHTML, t0: performance.now() };
      }

      async function performEncryption(raw, originalBtnHTML){
        try {
          const { key, encResult } = await encryptSecret(raw);
          return { keyBytes: key, encResult };
        } catch(e){
          console.error('[gone] encryption failed', e);
          resetButton(originalBtnHTML);
          return null;
        }
      }

      async function performUpload(encResult, keyBytes, ttl, originalBtnHTML){
        try {
          const res = await uploadCiphertext(encResult, keyBytes, ttl);
          if (!res) failureDelayReset(originalBtnHTML, 1200);
          return res;
        } catch(e){
          console.error('[gone] upload failed', e);
          setStatus('Network Err');
          failureDelayReset(originalBtnHTML, 1500);
          return null;
        }
      }

      function finalize(uploadRes, keyBytes, t0){
        logTotal(t0);
        const shareURL = buildShareURL(uploadRes.json.ID, uploadRes.version, keyBytes);
        buildAndShowResultPanel({ shareURL, expiresAt: uploadRes.json.expires_at, replaceTarget: cardSection });
      }

      const prep = prepareSubmission();
      if (!prep) return;
      const { raw, ttl, originalBtnHTML, t0 } = prep;
      const encryption = await performEncryption(raw, originalBtnHTML);
      if (!encryption) return;
      secureWipe(raw);
      const uploadRes = await performUpload(encryption.encResult, encryption.keyBytes, ttl, originalBtnHTML);
      if (!uploadRes) return;
      finalize(uploadRes, encryption.keyBytes, t0);
    }

    form.addEventListener('submit', handleSubmit);

      // Developer preview: ?preview=result shows a mock panel without submission
      (function previewCheck(){
        const params = new URLSearchParams(location.search);
        if (params.get('preview') === 'result') {
          const mockID = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
          const mockKey = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';
          const version = window.goneCrypto.version;
          const mockURL = `${location.origin}/secret/${mockID}#v${version}:${mockKey}`;
          const future = new Date(Date.now() + 30*60*1000).toISOString();
          buildAndShowResultPanel({ shareURL: mockURL, expiresAt: future, replaceTarget: form.closest('.card'), focus: false });
        }
      })();
  })();

    // Builds and replaces a target card with result panel.
    function buildAndShowResultPanel(opts){
      const { shareURL, expiresAt, replaceTarget, focus = true } = opts;
      const BACK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-left-icon lucide-arrow-left"><path d="m12 19-7-7 7-7"/><path d="M19 12H5"/></svg>';
      const COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';
      const CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
      const panel = document.createElement('div');
      const outer = document.createElement('div'); outer.id='result-outer'; panel.appendChild(outer);
      const h2 = document.createElement('h2'); h2.className='underline'; h2.textContent='Share This Link'; outer.appendChild(h2);
      const warnP = document.createElement('p'); warnP.className='security-warning-card'; warnP.innerHTML='<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4"/><path d="M12 17h.01"/><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/></svg>';
      const warnSpan=document.createElement('span'); warnSpan.textContent='Anyone with this link can view the secret exactly once.'; warnP.appendChild(warnSpan); outer.appendChild(warnP);
      const card=document.createElement('div'); card.className='card'; outer.appendChild(card);
      const hintP=document.createElement('p'); hintP.className='hint'; card.appendChild(hintP);
      const span=document.createElement('span'); hintP.appendChild(span); span.appendChild(document.createTextNode('Expires at '));
      const timeEl=document.createElement('time'); timeEl.setAttribute('datetime', expiresAt); timeEl.textContent=new Date(expiresAt).toLocaleString(); span.appendChild(timeEl);
      const input=document.createElement('input'); input.className='share-link'; input.id='share-link'; input.type='text'; input.readOnly=true; input.value=shareURL; card.appendChild(input);
      const actions=document.createElement('div'); actions.className='result-actions'; card.appendChild(actions);
      const backLink=document.createElement('a'); backLink.href='/'; backLink.className='back-link'; backLink.innerHTML=BACK_ICON + ' Create Another'; actions.appendChild(backLink);
      const copyBtn=document.createElement('button'); copyBtn.type='button'; copyBtn.className='copy-primary-btn'; copyBtn.setAttribute('aria-label','Copy full share link'); copyBtn.innerHTML='Copy Link ' + COPY_ICON; actions.appendChild(copyBtn);
      if (replaceTarget) replaceTarget.replaceWith(panel); else document.body.appendChild(panel);
      const shareInput = panel.querySelector('#share-link');
      const copyBtnRoot = panel.querySelector('.copy-primary-btn');
      if (focus && shareInput) shareInput.focus();
      if (copyBtnRoot) {
        copyBtnRoot.addEventListener('click', async function(){
          let success = true;
          try {
            await navigator.clipboard.writeText(shareURL);
          } catch(_) {
            // Minimal non-deprecated fallback: select text and instruct user.
            if (shareInput) {
              shareInput.focus();
              shareInput.select();
            }
            success = false;
          }
          if (success) {
            copyBtnRoot.innerHTML = 'Copied! ' + CHECK_ICON;
            copyBtnRoot.classList.add('copied');
            copyBtnRoot.disabled = true;
            setTimeout(function(){
              copyBtnRoot.innerHTML = 'Copy Link ' + COPY_ICON;
              copyBtnRoot.classList.remove('copied');
              copyBtnRoot.disabled = false;
            }, 2200);
          } else {
            // Notify user via alert instead of altering button text.
            alert('Copy failed. Please press \u2318/Ctrl+C to copy manually.');
          }
          return success;
        });
      }
      // no standalone newBtn today; the back-link handles create-another
      console.log('[gone] result panel shown');
    }

    // Secret consumption (decrypt) flow for /secret/{id} pages.
    (function consumeFlow(){
      if (!window.goneCrypto) return;
      const container = document.getElementById('secret-consume');
      if (!container) return; // not on consume page
      const statusEl = document.getElementById('secret-status');
      const pre = document.getElementById('secret-plaintext');
      const actions = document.getElementById('secret-actions');
      const copyBtn = document.getElementById('copy-secret');

      function setStatus(msg){ if (statusEl) statusEl.textContent = msg; }
      function logTiming(label,start,end){ console.log(`[gone][timing] ${label}: ${(end-start).toFixed(2)}ms`); }

      // Parse fragment: #v<version>:<key>
      const hash = location.hash || '';
      const fragMatch = /^#v(\d+):([A-Za-z0-9_-]{10,})$/.exec(hash);
      if (!fragMatch) {
        setStatus('Missing or invalid key fragment. Cannot decrypt.');
        return;
      }
      const fragVersion = parseInt(fragMatch[1], 10);
      const keyB64 = fragMatch[2];
      if (fragVersion !== window.goneCrypto.version) {
        setStatus('Unsupported version');
        return;
      }

      // Extract ID from path /secret/{id}
      const pathParts = location.pathname.split('/');
      const id = pathParts[pathParts.length-1];
      if (!id) { setStatus('Invalid secret id'); return; }

      async function fetchSecret(secretID) {
        setStatus('Fetching…');
        const tFetchStart = performance.now();
        const resp = await fetch(`/api/secret/${secretID}`);
        const tFetchEnd = performance.now();
        logTiming('consume_fetch', tFetchStart, tFetchEnd);
        if (!resp.ok) {
          setStatus(resp.status === 404 ? 'Secret not found or already consumed.' : 'Fetch error');
          return null;
        }
        return resp;
      }

      function validateHeaders(resp) {
        const versionHdr = parseInt(resp.headers.get('X-Gone-Version')||'0', 10);
        if (versionHdr !== window.goneCrypto.version) {
          setStatus('Version mismatch');
          return null;
        }
        const nonceB64 = resp.headers.get('X-Gone-Nonce') || '';
        return nonceB64;
      }

      async function decryptPayload(resp, nonceB64, fragmentKeyB64) {
        const nonce = window.goneCrypto.b64urlDecode(nonceB64);
        const ctBuf = new Uint8Array(await resp.arrayBuffer());
        const keyBytes = window.goneCrypto.importKeyB64(fragmentKeyB64);
        const additionalData = new TextEncoder().encode('gone:v1');
        const cryptoKey = await crypto.subtle.importKey('raw', keyBytes, {name:'AES-GCM'}, false, ['decrypt']);
        try {
          const tDecStart = performance.now();
          const ptBuf = await crypto.subtle.decrypt({name:'AES-GCM', iv: nonce, additionalData}, cryptoKey, ctBuf);
          const tDecEnd = performance.now();
          logTiming('consume_decrypt', tDecStart, tDecEnd);
          return new TextDecoder().decode(ptBuf);
        } catch(_) {
          setStatus('Decryption failed');
          return null;
        }
      }

      function showPlaintext(text) {
        if (pre) {
          pre.textContent = text;
          pre.hidden = false;
        }
        if (actions) actions.hidden = false;
        setStatus('Decrypted');
        if (copyBtn) {
          copyBtn.addEventListener('click', async ()=>{
            try {
              await navigator.clipboard.writeText(text);
              copyBtn.textContent='Copied!';
              setTimeout(()=>copyBtn.textContent='Copy Secret', 2500);
            } catch(_) {
              copyBtn.textContent='Copy failed';
              setTimeout(()=>copyBtn.textContent='Copy Secret', 2500);
            }
          });
        }
      }

      (async function runConsume(){
        try {
          const resp = await fetchSecret(id);
          if (!resp) return;
          const nonceB64 = validateHeaders(resp);
          if (!nonceB64) return;
          const tStartTotal = performance.now();
          const plaintext = await decryptPayload(resp, nonceB64, keyB64);
          if (!plaintext) return;
          showPlaintext(plaintext);
          const tEndTotal = performance.now();
          logTiming('consume_total', tStartTotal, tEndTotal);
        } catch(e) {
          console.error('[gone] consume error', e);
          setStatus('Unexpected error');
        }
      })();
    })();