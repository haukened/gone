'use strict';

// Theme persistence & security warning logic extracted from app.js
(function () {
  const STORAGE_KEY = 'gone.theme';
  const checkbox = document.getElementById('theme-switch');
  if (!checkbox) return false;
  if (typeof window === 'undefined') return false;
  const desc = document.getElementById('theme-switch-desc');

  function systemPrefersDark() {
    try { if (typeof window !== 'undefined' && window.matchMedia) { return !!window.matchMedia('(prefers-color-scheme: dark)').matches; } } catch (_) { return false; }
    return false;
  }
  function applyTheme(mode, persistParam) {
    const persist = (persistParam === undefined) ? true : persistParam;
    const isDark = mode === 'dark';
    checkbox.checked = isDark;
    checkbox.setAttribute('aria-pressed', String(isDark));
    if (desc) {
      desc.textContent = isDark ? 'Toggle theme. Currently dark; activates light mode.' : 'Toggle theme. Currently light; activates dark mode.';
    }
    if (persist) {
      try { localStorage.setItem(STORAGE_KEY, mode); } catch (_) { return false; }
    }
    return true;
  }
  function loadInitialTheme() {
    let stored = '';
    try { stored = localStorage.getItem(STORAGE_KEY); } catch (_) { return false; }
    if (stored !== 'light' && stored !== 'dark') {
      stored = systemPrefersDark() ? 'dark' : 'light';
      try { localStorage.setItem(STORAGE_KEY, stored); } catch (_) { return false; }
    }
    applyTheme(stored, false);
    return true;
  }
  checkbox.addEventListener('change', function () {
    const mode = checkbox.checked ? 'dark' : 'light';
    applyTheme(mode, true);
    return true;
  });
  if (!localStorage.getItem(STORAGE_KEY) && window.matchMedia) {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    mq.addEventListener('change', function (e) {
      if (!localStorage.getItem(STORAGE_KEY)) {
        applyTheme(e.matches ? 'dark' : 'light', true);
      }
      return true;
    });
  }
  loadInitialTheme();
  (function securityWarning() {
    try {
      const insecure = window.location.protocol === 'http:';
      if (insecure) {
        const section = document.querySelector('.security-warning');
        if (section) {
          section.hidden = false;
          section.setAttribute('aria-hidden', 'false');
        }
      }
    } catch (_) { return false; }
    return true;
  })();
  console.log('Gone theme module loaded');
  return true;
})();
