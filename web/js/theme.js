'use strict';

// Theme handling and the insecure-connection warning. Loaded without defer
// so a stored theme is applied before first paint. With nothing stored the
// page follows the system preference through CSS light-dark(); a click on
// #theme-toggle stores an explicit choice. Exposed as window.goneTheme.
(function themeModule() {
  if (window.goneTheme) return;
  const STORAGE_KEY = 'gone.theme';
  const DARK_QUERY = '(prefers-color-scheme: dark)';
  const root = document.documentElement;

  function readStored() {
    try {
      const v = localStorage.getItem(STORAGE_KEY);
      return v === 'light' || v === 'dark' ? v : '';
    } catch {
      return '';
    }
  }

  function store(mode) {
    try {
      localStorage.setItem(STORAGE_KEY, mode);
    } catch { /* private mode: the choice lasts for this page only */ }
  }

  function systemDark() {
    return Boolean(window.matchMedia) && window.matchMedia(DARK_QUERY).matches;
  }

  // effective returns the theme currently shown: the applied choice, else the system's.
  function effective() {
    return root.dataset.theme || (systemDark() ? 'dark' : 'light');
  }

  function apply(mode) {
    if (mode) root.dataset.theme = mode;
  }

  function syncToggle(btn) {
    btn.setAttribute('aria-pressed', String(effective() === 'dark'));
  }

  // toggle switches between light and dark, stores the choice, and updates btn.
  function toggle(btn) {
    const next = effective() === 'dark' ? 'light' : 'dark';
    apply(next);
    store(next);
    syncToggle(btn);
  }

  function wireToggle() {
    const btn = document.getElementById('theme-toggle');
    if (!btn) return;
    btn.hidden = false;
    syncToggle(btn);
    btn.addEventListener('click', function () { toggle(btn); });
    if (window.matchMedia) {
      window.matchMedia(DARK_QUERY).addEventListener('change', function () { syncToggle(btn); });
    }
  }

  // warnIfInsecure reveals every .security-warning when served over plain HTTP.
  function warnIfInsecure() {
    if (window.location.protocol !== 'http:') return;
    console.warn('[gone] insecure context detected (HTTP)');
    document.querySelectorAll('.security-warning').forEach(function (n) { n.hidden = false; });
  }

  function init() {
    wireToggle();
    warnIfInsecure();
  }

  apply(readStored());
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
  window.goneTheme = Object.freeze({ effective: effective, toggle: toggle });
})();
