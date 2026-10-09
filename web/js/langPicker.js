'use strict';

// Language picker: the globe pill beside the theme toggle. It is a native
// <select> listing each language in its own name, laid over the pill so it
// keeps the platform's accessible picker. Choosing a language switches the
// page in place through window.goneI18n; if that language can't be loaded,
// the choice is undone and announced. Shown only when scripts run and more
// than one language is available. Exposed as window.goneLangPicker.
(function langPickerModule() {
  if (window.goneLangPicker || !window.goneI18n) return;
  const i18n = window.goneI18n;

  // codeFor returns the short code shown on the pill, e.g. "PT" for pt-BR.
  function codeFor(tag) {
    return tag.split('-')[0].toUpperCase();
  }

  function sync(select, code) {
    select.value = i18n.locale;
    if (code) code.textContent = codeFor(i18n.locale);
  }

  async function change(select, code, status) {
    select.setAttribute('aria-busy', 'true');
    const ok = await i18n.setLocale(select.value);
    select.removeAttribute('aria-busy');
    sync(select, code);
    if (ok) i18n.clear(status);
    else i18n.set(status, 'js.lang.failed');
  }

  // wire shows the picker and handles changes. Safe to call more than once.
  function wire() {
    const select = document.getElementById('lang-select');
    const wrap = select ? select.closest('.lang-pick') : null;
    if (!wrap || select.dataset.wired || i18n.locales().length < 2) return;
    select.dataset.wired = '1';
    const code = wrap.querySelector('.lang-code');
    const status = document.getElementById('lang-status');
    sync(select, code);
    select.addEventListener('change', function () { change(select, code, status); });
    i18n.onChange(function () { sync(select, code); });
    wrap.hidden = false;
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', wire);
  else wire();

  window.goneLangPicker = Object.freeze({ wire: wire });
})();
