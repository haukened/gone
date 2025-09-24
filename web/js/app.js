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
	var checkbox = document.getElementById('theme-switch');
	if (!checkbox) {
		return false;
	}

	// guard against server-side rendering (e.g. static export)
	if (typeof window === 'undefined') { return false; }

	var desc = document.getElementById('theme-switch-desc');

	function systemPrefersDark() {
		try {
			if (typeof window !== 'undefined' && window.matchMedia) {
				return !!window.matchMedia('(prefers-color-scheme: dark)').matches;
			}
		} catch(_) { return false; }
		return false;
	}

	function applyTheme(mode, persistParam) {
		var persist = (persistParam === undefined) ? true : persistParam;
		var isDark = mode === 'dark';
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
		var stored = "";
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
		var mode = checkbox.checked ? 'dark' : 'light';
		applyTheme(mode, true);
		return true;
	});

	// React to system preference changes if user has not explicitly overridden (only when no stored value).
	if (!localStorage.getItem(STORAGE_KEY) && window.matchMedia) {
		var mq = window.matchMedia('(prefers-color-scheme: dark)');
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
			var insecure = window.location.protocol === 'http:';
			if (insecure) {
				var section = document.querySelector('.security-warning');
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
	var ta = document.getElementById('secret');
	if (!ta) return false;
  var max = 40 * 16; // 40rem assuming 16px root, adjust as needed
	function grow() {
    ta.style.height = 'auto';
    var next = Math.min(ta.scrollHeight, max);
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
	var VERSION = 0x01; // increment for future formats
	var KEY_BYTES = 32; // 256-bit AES-GCM
	var NONCE_BYTES = 12; // 96-bit nonce per NIST recommendation

	if (window.goneCrypto) {
		return; // already defined
	}

	// Utilities
	function utf8Encode(str) { return new TextEncoder().encode(str); }
	function b64urlEncode(bytes) {
		var bin = '';
		for (var i=0;i<bytes.length;i++) bin += String.fromCharCode(bytes[i]);
		var b64 = btoa(bin).replace(/\+/g,'-').replace(/\//g,'_');
		var end = b64.length; // manual trim '=' padding instead of regex quantifier
		while (end > 0 && b64.charAt(end-1) === '=') end--;
		return b64.substring(0, end);
	}
	function b64urlDecode(s) {
		var norm = s.replace(/-/g,'+').replace(/_/g,'/');
		while (norm.length % 4) norm += '=';
		var bin = atob(norm);
		var out = new Uint8Array(bin.length);
		for (var i=0;i<bin.length;i++) out[i] = bin.charCodeAt(i);
		return out;
	}
	function randomBytes(n) {
		var b = new Uint8Array(n);
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
		var key = await importKey(keyBytes);
		var nonce = randomBytes(NONCE_BYTES);
		var additionalData = new TextEncoder().encode('gone:v1');
		var ctBuf = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce, additionalData }, key, plaintextBytes);
		var ct = new Uint8Array(ctBuf);
			return { nonce, ciphertext: ct };
	}

	function exportKeyB64(keyBytes) { return b64urlEncode(keyBytes); }
	function importKeyB64(k) { var b = b64urlDecode(k); if (b.length !== KEY_BYTES) throw new Error('invalid key'); return b; }

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
		var form = document.getElementById('create-secret');
		if (!form || !window.goneCrypto) return;
		var textarea = document.getElementById('secret');
		var ttlSelect = document.getElementById('ttl');
		var primaryBtn = form.querySelector('button[type="submit"]');
		var cardSection = form.closest('.card');
		if (!textarea || !ttlSelect || !primaryBtn || !cardSection) return;

		function logTiming(label, start, end){
			var ms = (end - start).toFixed(2);
			console.log(`[gone][timing] ${label}: ${ms}ms`);
		}

		async function handleSubmit(ev){
			ev.preventDefault();
			var raw = textarea.value;
			if (!raw) {
				console.warn('[gone] empty secret submission blocked');
				return;
			}
			var ttl = ttlSelect.value; // server expects parseable duration label
			primaryBtn.disabled = true;
			var originalBtnHTML = primaryBtn.innerHTML;
			function setStatus(msg){ primaryBtn.textContent = msg; return true; }
			var t0 = performance.now();
			let keyBytes, encResult;
			try {
				keyBytes = window.goneCrypto.generateKey();
				var encStart = performance.now();
				encResult = await window.goneCrypto.encrypt(raw, keyBytes);
				var encEnd = performance.now();
				logTiming('encrypt', encStart, encEnd);
			} catch(e){
				console.error('[gone] encryption failed', e);
				primaryBtn.disabled = false;
				primaryBtn.textContent = originalBtnHTML;
				return;
			}
			// Secure-ish wipe of textarea content
			try {
				var len = raw.length;
				textarea.value = ''.padEnd(len, '\u2022');
				textarea.value = '';
			} catch(_) {}

			// Build request
			var { nonce, ciphertext } = encResult;
			var version = window.goneCrypto.version;
			var nonceB64 = window.goneCrypto.b64urlEncode(nonce);
			var uploadStart = performance.now();
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
				var uploadEnd = performance.now();
				logTiming('upload', uploadStart, uploadEnd);
				if (!resp.ok) {
					console.error('[gone] server error', resp.status);
					setStatus('Error');
					setTimeout(function(){ primaryBtn.disabled = false; primaryBtn.textContent = originalBtnHTML; }, 1200);
					return;
				}
				json = await resp.json();
			} catch(e){
				var uploadEnd = performance.now();
				logTiming('upload_error', uploadStart, uploadEnd);
				console.error('[gone] upload failed', e);
				setStatus('Network Err');
				setTimeout(function(){ primaryBtn.disabled = false; primaryBtn.textContent = originalBtnHTML; }, 1500);
				return;
			}
			var t1 = performance.now();
			logTiming('total_submit_cycle', t0, t1);

			// varruct share URL
			var keyB64 = window.goneCrypto.exportKeyB64(keyBytes);
			var shareURL = `${location.origin}/secret/${json.ID}#v${version}:${keyB64}`;

				buildAndShowResultPanel({ shareURL, expiresAt: json.expires_at, replaceTarget: cardSection });
		}

		form.addEventListener('submit', handleSubmit);

			// Developer preview: ?preview=result shows a mock panel without submission
			(function previewCheck(){
				var params = new URLSearchParams(location.search);
				if (params.get('preview') === 'result') {
					var mockID = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
					var mockKey = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';
					var version = window.goneCrypto.version;
					var mockURL = `${location.origin}/secret/${mockID}#v${version}:${mockKey}`;
					var future = new Date(Date.now() + 30*60*1000).toISOString();
					buildAndShowResultPanel({ shareURL: mockURL, expiresAt: future, replaceTarget: form.closest('.card'), focus: false });
				}
			})();
	})();

		// Builds and replaces a target card with result panel.
		function buildAndShowResultPanel(opts){
			var { shareURL, expiresAt, replaceTarget, focus = true } = opts;
      var BACK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-left-icon lucide-arrow-left"><path d="m12 19-7-7 7-7"/><path d="M19 12H5"/></svg>'
			var COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';
			var CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
			var panel = document.createElement('div');
			var outer = document.createElement('div'); outer.id='result-outer'; panel.appendChild(outer);
			var h2 = document.createElement('h2'); h2.className='underline'; h2.textContent='Share This Link'; outer.appendChild(h2);
			var warnP = document.createElement('p'); warnP.className='security-warning-card'; warnP.innerHTML='<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4"/><path d="M12 17h.01"/><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/></svg>';
			var warnSpan=document.createElement('span'); warnSpan.textContent='Anyone with this link can view the secret exactly once.'; warnP.appendChild(warnSpan); outer.appendChild(warnP);
			var card=document.createElement('div'); card.className='card'; outer.appendChild(card);
			var hintP=document.createElement('p'); hintP.className='hint'; card.appendChild(hintP);
			var span=document.createElement('span'); hintP.appendChild(span); span.appendChild(document.createTextNode('Expires at '));
			var timeEl=document.createElement('time'); timeEl.setAttribute('datetime', expiresAt); timeEl.textContent=new Date(expiresAt).toLocaleString(); span.appendChild(timeEl);
			var input=document.createElement('input'); input.className='share-link'; input.id='share-link'; input.type='text'; input.readOnly=true; input.value=shareURL; card.appendChild(input);
			var actions=document.createElement('div'); actions.className='result-actions'; card.appendChild(actions);
			var backLink=document.createElement('a'); backLink.href='/'; backLink.className='back-link'; backLink.innerHTML=BACK_ICON + ' Create Another'; actions.appendChild(backLink);
			var copyBtn=document.createElement('button'); copyBtn.type='button'; copyBtn.className='copy-primary-btn'; copyBtn.setAttribute('aria-label','Copy full share link'); copyBtn.innerHTML='Copy Link ' + COPY_ICON; actions.appendChild(copyBtn);
			if (replaceTarget) replaceTarget.replaceWith(panel); else document.body.appendChild(panel);
			var shareInput = panel.querySelector('#share-link');
			var copyBtn = panel.querySelector('.copy-primary-btn');
			if (focus && shareInput) shareInput.focus();
			if (copyBtn) {
				copyBtn.addEventListener('click', function(){
					var success = true;
					try { navigator.clipboard.writeText(shareURL); } catch(_) { try { shareInput.select(); document.execCommand('copy'); } catch(_) { success = false; } }
					if (success) {
						copyBtn.dataset.original = copyBtn.textContent;
						copyBtn.textContent = 'Copied!';
						copyBtn.classList.add('copied');
						copyBtn.disabled = true;
						setTimeout(function(){
							copyBtn.textContent = copyBtn.dataset.original || 'Copy Link';
							copyBtn.classList.remove('copied');
							copyBtn.disabled = false;
						}, 2200);
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
			var container = document.getElementById('secret-consume');
			if (!container) return; // not on consume page
			var statusEl = document.getElementById('secret-status');
			var pre = document.getElementById('secret-plaintext');
			var actions = document.getElementById('secret-actions');
			var copyBtn = document.getElementById('copy-secret');

			function setStatus(msg){ if (statusEl) statusEl.textContent = msg; }
			function logTiming(label,start,end){ console.log(`[gone][timing] ${label}: ${(end-start).toFixed(2)}ms`); }

			// Parse fragment: #v<version>:<key>
			var hash = location.hash || '';
			var fragMatch = /^#v(\d+):([A-Za-z0-9_-]{10,})$/.exec(hash);
			if (!fragMatch) {
				setStatus('Missing or invalid key fragment. Cannot decrypt.');
				return;
			}
			var fragVersion = parseInt(fragMatch[1], 10);
			var keyB64 = fragMatch[2];
			if (fragVersion !== window.goneCrypto.version) {
				setStatus('Unsupported version');
				return;
			}

			// Extract ID from path /secret/{id}
			var pathParts = location.pathname.split('/');
			var id = pathParts[pathParts.length-1];
			if (!id) { setStatus('Invalid secret id'); return; }

			(async () => {
				try {
					setStatus('Fetching…');
					var tFetchStart = performance.now();
					var resp = await fetch(`/api/secret/${id}`);
					var tFetchEnd = performance.now();
					logTiming('consume_fetch', tFetchStart, tFetchEnd);
					if (!resp.ok) {
						setStatus(resp.status === 404 ? 'Secret not found or already consumed.' : 'Fetch error');
						return;
					}
					var nonceB64 = resp.headers.get('X-Gone-Nonce');
					var versionHdr = parseInt(resp.headers.get('X-Gone-Version')||'0', 10);
					if (versionHdr !== window.goneCrypto.version) {
						setStatus('Version mismatch');
						return;
					}
					var nonce = window.goneCrypto.b64urlDecode(nonceB64 || '');
					var ctBuf = new Uint8Array(await resp.arrayBuffer());
					var tDecStart = performance.now();
					// Revarruct envelope bytes to reuse decrypt() expecting envelope format? We switched to raw.
					// Instead replicate decrypt: import key and call subtle.decrypt directly.
					var keyBytes = window.goneCrypto.importKeyB64(keyB64);
					var additionalData = new TextEncoder().encode('gone:v1');
					var cryptoKey = await crypto.subtle.importKey('raw', keyBytes, {name:'AES-GCM'}, false, ['decrypt']);
					let ptBuf;
					try {
						ptBuf = await crypto.subtle.decrypt({name:'AES-GCM', iv: nonce, additionalData}, cryptoKey, ctBuf);
					} catch(e) {
						setStatus('Decryption failed');
						return;
					}
					var tDecEnd = performance.now();
					logTiming('consume_decrypt', tDecStart, tDecEnd);
					logTiming('consume_total', tFetchStart, tDecEnd);
					var plaintext = new TextDecoder().decode(ptBuf);
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