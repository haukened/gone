"use strict";
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
		return; // safety: markup missing
	}

	const desc = document.getElementById('theme-switch-desc');

	function systemPrefersDark() {
		return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
	}

	function loadInitialTheme() {
		let stored = null;
		try {
			stored = localStorage.getItem(STORAGE_KEY);
		} catch (_) { /* ignore storage errors (e.g., privacy mode) */ }

		if (stored !== 'light' && stored !== 'dark') {
			stored = systemPrefersDark() ? 'dark' : 'light';
			try { localStorage.setItem(STORAGE_KEY, stored); } catch (_) { /* ignore */ }
		}
		applyTheme(stored, false);
	}

	function applyTheme(mode, persist = true) {
		const isDark = mode === 'dark';
		checkbox.checked = isDark; // checked => dark variables active (CSS :has)
		checkbox.setAttribute('aria-pressed', String(isDark));
		if (desc) {
			desc.textContent = isDark
				? 'Toggle theme. Currently dark; activates light mode.'
				: 'Toggle theme. Currently light; activates dark mode.';
		}
		if (persist) {
			try { localStorage.setItem(STORAGE_KEY, mode); } catch (_) { /* ignore */ }
		}
	}

	checkbox.addEventListener('change', () => {
		const mode = checkbox.checked ? 'dark' : 'light';
		applyTheme(mode, true);
	});

	// React to system preference changes if user has not explicitly overridden (only when no stored value).
	if (!localStorage.getItem(STORAGE_KEY) && window.matchMedia) {
		const mq = window.matchMedia('(prefers-color-scheme: dark)');
		mq.addEventListener('change', (e) => {
			if (!localStorage.getItem(STORAGE_KEY)) {
				applyTheme(e.matches ? 'dark' : 'light', true);
			}
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
		} catch(_) { /* ignore */ }
	})();

	// Placeholder for future client-side encryption workflow bootstrapping.
	console.log('Gone UI loaded');
})();

(function autoResize() {
  const ta = document.getElementById('secret');
  if (!ta) return;
  const max = 40 * 16; // 40rem assuming 16px root, adjust as needed
  function grow() {
    ta.style.height = 'auto';
    const next = Math.min(ta.scrollHeight, max);
    ta.style.height = next + 'px';
    ta.style.overflowY = ta.scrollHeight > max ? 'auto' : 'hidden';
  }
  ta.addEventListener('input', grow);
  grow();
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
	function utf8Encode(str) {
		return new TextEncoder().encode(str);
	}
	function utf8Decode(buf) {
		return new TextDecoder().decode(buf);
	}
	function b64urlEncode(bytes) {
		let bin = '';
		for (let i=0;i<bytes.length;i++) bin += String.fromCharCode(bytes[i]);
		return btoa(bin).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
	}
	function b64urlDecode(s) {
		s = s.replace(/-/g,'+').replace(/_/g,'/');
		// pad
		while (s.length % 4) s += '=';
		const bin = atob(s);
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

		async function handleSubmit(ev){
			ev.preventDefault();
			const raw = textarea.value;
			if (!raw) {
				console.warn('[gone] empty secret submission blocked');
				return;
			}
			const ttl = ttlSelect.value; // server expects parseable duration label
			primaryBtn.disabled = true;
			const originalBtnHTML = primaryBtn.innerHTML;
			function setStatus(msg){ primaryBtn.innerHTML = `<span>${msg}</span>`; }
			const t0 = performance.now();
			let keyBytes, encResult;
			try {
				keyBytes = window.goneCrypto.generateKey();
				const encStart = performance.now();
				encResult = await window.goneCrypto.encrypt(raw, keyBytes);
				const encEnd = performance.now();
				logTiming('encrypt', encStart, encEnd);
			} catch(e){
				console.error('[gone] encryption failed', e);
				primaryBtn.disabled = false;
				primaryBtn.innerHTML = originalBtnHTML;
				return;
			}
			// Secure-ish wipe of textarea content
			try {
				const len = raw.length;
				textarea.value = ''.padEnd(len, '\u2022');
				textarea.value = '';
			} catch(_) {}

			// Build request
			const { nonce, ciphertext } = encResult;
			const version = window.goneCrypto.version;
			const nonceB64 = window.goneCrypto.b64urlEncode(nonce);
			const uploadStart = performance.now();
			setStatus('Uploading…');
			let resp, json;
			try {
				resp = await fetch('/api/secret', {
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
					setTimeout(()=>{ primaryBtn.disabled = false; primaryBtn.innerHTML = originalBtnHTML; }, 1200);
					return;
				}
				json = await resp.json();
			} catch(e){
				const uploadEnd = performance.now();
				logTiming('upload_error', uploadStart, uploadEnd);
				console.error('[gone] upload failed', e);
				setStatus('Network Err');
				setTimeout(()=>{ primaryBtn.disabled = false; primaryBtn.innerHTML = originalBtnHTML; }, 1500);
				return;
			}
			const t1 = performance.now();
			logTiming('total_submit_cycle', t0, t1);

			// Construct share URL
			const keyB64 = window.goneCrypto.exportKeyB64(keyBytes);
			const shareURL = `${location.origin}/secret/${json.ID}#v${version}:${keyB64}`;

				buildAndShowResultPanel({ shareURL, expiresAt: json.expires_at, replaceTarget: cardSection });
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
      const BACK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-left-icon lucide-arrow-left"><path d="m12 19-7-7 7-7"/><path d="M19 12H5"/></svg>'
			const COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';
			const CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
			const panel = document.createElement('div');
			panel.innerHTML = `
        <div id="result-outer">
          <h2 class="underline">Share This Link</h2>
          <p class="security-warning-card">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4"/><path d="M12 17h.01"/><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/></svg>
            <span>Anyone with this link can view the secret exactly once.</span>
          </p>
          <div class="card">
            <p class="hint">
              <span>Expires at <time datetime="${expiresAt}">${new Date(expiresAt).toLocaleString()}</time></span>
            </p>
            <input class="share-link" id="share-link" type="text" readonly value="${shareURL}" />
            <div class="result-actions">
              <a href="/" class="back-link">${BACK_ICON} Create Another</a>
              <button type="button" class="copy-primary-btn" aria-label="Copy full share link">Copy Link ${COPY_ICON}</button>
            </div>
          </div>
        </div>
      `;
			if (replaceTarget) replaceTarget.replaceWith(panel); else document.body.appendChild(panel);
			const shareInput = panel.querySelector('#share-link');
			const copyBtn = panel.querySelector('.copy-primary-btn');
			if (focus && shareInput) shareInput.focus();
			if (copyBtn) {
				copyBtn.addEventListener('click', async () => {
					let success = true;
					try {
						await navigator.clipboard.writeText(shareURL);
					} catch(_) {
						try { shareInput.select(); document.execCommand('copy'); } catch(_) { success = false; }
					}
					if (success) {
						copyBtn.dataset.original = copyBtn.innerHTML;
						copyBtn.innerHTML = 'Copied ' + CHECK_ICON;
						copyBtn.classList.add('copied');
						copyBtn.disabled = true;
						setTimeout(()=>{ 
							copyBtn.innerHTML = copyBtn.dataset.original || ('Copy Link ' + COPY_ICON);
							copyBtn.classList.remove('copied');
							copyBtn.disabled = false;
						}, 2200);
					}
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

			(async () => {
				try {
					setStatus('Fetching…');
					const tFetchStart = performance.now();
					const resp = await fetch(`/api/secret/${id}`);
					const tFetchEnd = performance.now();
					logTiming('consume_fetch', tFetchStart, tFetchEnd);
					if (!resp.ok) {
						setStatus(resp.status === 404 ? 'Secret not found or already consumed.' : 'Fetch error');
						return;
					}
					const nonceB64 = resp.headers.get('X-Gone-Nonce');
					const versionHdr = parseInt(resp.headers.get('X-Gone-Version')||'0', 10);
					if (versionHdr !== window.goneCrypto.version) {
						setStatus('Version mismatch');
						return;
					}
					const nonce = window.goneCrypto.b64urlDecode(nonceB64 || '');
					const ctBuf = new Uint8Array(await resp.arrayBuffer());
					const tDecStart = performance.now();
					// Reconstruct envelope bytes to reuse decrypt() expecting envelope format? We switched to raw.
					// Instead replicate decrypt: import key and call subtle.decrypt directly.
					const keyBytes = window.goneCrypto.importKeyB64(keyB64);
					const additionalData = new TextEncoder().encode('gone:v1');
					const cryptoKey = await crypto.subtle.importKey('raw', keyBytes, {name:'AES-GCM'}, false, ['decrypt']);
					let ptBuf;
					try {
						ptBuf = await crypto.subtle.decrypt({name:'AES-GCM', iv: nonce, additionalData}, cryptoKey, ctBuf);
					} catch(e) {
						setStatus('Decryption failed');
						return;
					}
					const tDecEnd = performance.now();
					logTiming('consume_decrypt', tDecStart, tDecEnd);
					logTiming('consume_total', tFetchStart, tDecEnd);
					const plaintext = new TextDecoder().decode(ptBuf);
					if (pre) {
						pre.textContent = plaintext;
						pre.hidden = false;
					}
					if (actions) actions.hidden = false;
					setStatus('Decrypted');
					copyBtn?.addEventListener('click', async ()=>{
						try {
							await navigator.clipboard.writeText(plaintext);
							copyBtn.textContent='Copied!';
							setTimeout(()=>copyBtn.textContent='Copy Secret', 2500);
						} catch(_) {
							copyBtn.textContent='Copy failed';
							setTimeout(()=>copyBtn.textContent='Copy Secret', 2500);
						}}
					);
				} catch(e) {
					console.error('[gone] consume error', e);
					setStatus('Unexpected error');
				}
			})();
		})();