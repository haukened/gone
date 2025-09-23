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
			const host = window.location.hostname;
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
// Envelope layout (base64url when transmitted):
// version(1 byte=0x01) || nonce(12 bytes) || ciphertext+tag (WebCrypto returns both together)
// Key (32 random bytes) is never sent to server; represented in URL fragment as: v1:<base64url(key)>
// Public API attached at window.goneCrypto { generateKey(), encrypt(plaintext|Uint8Array), decrypt(envelopeB64, keyB64) }
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

	function assembleEnvelope(nonce, ct) {
		const out = new Uint8Array(1 + nonce.length + ct.length);
		out[0] = VERSION;
		out.set(nonce, 1);
		out.set(ct, 1 + nonce.length);
		return out;
	}

	function parseEnvelope(bytes) {
		if (bytes.length < 1 + NONCE_BYTES + 16) { // tag is 16 bytes
			throw new Error('envelope too short');
		}
		const version = bytes[0];
		if (version !== VERSION) {
			throw new Error('unsupported version: ' + version);
		}
		const nonce = bytes.slice(1, 1 + NONCE_BYTES);
		const ct = bytes.slice(1 + NONCE_BYTES);
		return { nonce, ct };
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

	async function decrypt(envelopeB64, keyB64) {
		const keyBytes = b64urlDecode(keyB64);
		if (keyBytes.length !== KEY_BYTES) {
			throw new Error('invalid key length');
		}
		const key = await importKey(keyBytes);
		const envBytes = b64urlDecode(envelopeB64);
		const { nonce, ct } = parseEnvelope(envBytes);
		const additionalData = new TextEncoder().encode('gone:v1');
		let ptBuf;
		try {
			ptBuf = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce, additionalData }, key, ct);
		} catch (e) {
			throw new Error('decryption failed');
		}
		return utf8Decode(new Uint8Array(ptBuf));
	}

	function exportKeyB64(keyBytes) { return b64urlEncode(keyBytes); }
	function importKeyB64(k) { const b = b64urlDecode(k); if (b.length !== KEY_BYTES) throw new Error('invalid key'); return b; }

	window.goneCrypto = {
		version: VERSION,
		generateKey,
		exportKeyB64,
		importKeyB64,
		encrypt,
		decrypt,
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

		function logPhase(label, start, end){
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
				logPhase('encrypt', encStart, encEnd);
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
				logPhase('upload', uploadStart, uploadEnd);
				if (!resp.ok) {
					console.error('[gone] server error', resp.status);
					setStatus('Error');
					setTimeout(()=>{ primaryBtn.disabled = false; primaryBtn.innerHTML = originalBtnHTML; }, 1200);
					return;
				}
				json = await resp.json();
			} catch(e){
				const uploadEnd = performance.now();
				logPhase('upload_error', uploadStart, uploadEnd);
				console.error('[gone] upload failed', e);
				setStatus('Network Err');
				setTimeout(()=>{ primaryBtn.disabled = false; primaryBtn.innerHTML = originalBtnHTML; }, 1500);
				return;
			}
			const t1 = performance.now();
			logPhase('total_submit_cycle', t0, t1);

			// Construct share URL
			const keyB64 = window.goneCrypto.exportKeyB64(keyBytes);
			const shareURL = `${location.origin}/secret/${json.ID}#v${version}:${keyB64}`;

			// Build result panel
			const panel = document.createElement('div');
			panel.className = 'card';
			panel.innerHTML = `
				<div class="result-panel">
					<h2>Secret Ready</h2>
					<p class="result-meta">Expires at <time datetime="${json.expires_at}">${new Date(json.expires_at).toLocaleString()}</time></p>
					<label class="sr-only" for="share-link">Share Link</label>
					<div class="share-link-wrap">
						<input id="share-link" type="text" readonly value="${shareURL}" />
						<button type="button" class="copy-btn">Copy Link</button>
					</div>
					<p class="result-warning">Anyone with this link can view the secret exactly once.</p>
					<div class="result-actions">
						<button type="button" class="primary new-btn">Create Another</button>
					</div>
				</div>`;
			cardSection.replaceWith(panel);
			const shareInput = panel.querySelector('#share-link');
			const copyBtn = panel.querySelector('.copy-btn');
			const newBtn = panel.querySelector('.new-btn');
			if (shareInput) shareInput.focus();
			copyBtn?.addEventListener('click', async ()=>{
				try { await navigator.clipboard.writeText(shareURL); copyBtn.textContent = 'Copied!'; setTimeout(()=>copyBtn.textContent='Copy Link', 2500); } catch(_) {
					shareInput.select(); document.execCommand('copy'); copyBtn.textContent='Copied*'; setTimeout(()=>copyBtn.textContent='Copy Link', 2500);
				}
			});
			newBtn?.addEventListener('click', ()=>{ location.href = '/'; });
		}

		form.addEventListener('submit', handleSubmit);
	})();