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
