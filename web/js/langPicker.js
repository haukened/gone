'use strict';

// Language picker: the globe pill beside the theme toggle. The pill is a
// disclosure button for a short list of buttons, one per language, each in
// its own name and marked with lang. The current language carries
// aria-current. Choosing one switches the page in place through
// window.goneI18n; if that language can't be loaded, the page stays as it was
// and the failure is announced. Arrow keys, Home and End move through the
// list; Escape closes it and returns to the pill; clicking or tabbing away
// closes it. Shown only when scripts run and more than one language is
// available. The list itself is langPickerMenu.js. Exposed as
// window.goneLangPicker.
(function langPickerModule() {
  if (window.goneLangPicker || !window.goneI18n || !window.goneLangPickerMenu) return;
  const i18n = window.goneI18n;
  const list = window.goneLangPickerMenu;
  const sync = list.sync;
  const isOpen = list.isOpen;
  const open = list.open;
  const close = list.close;

  async function choose(p, tag) {
    close(p, true);
    if (tag === i18n.locale) return;
    p.button.setAttribute('aria-busy', 'true');
    const ok = await i18n.setLocale(tag);
    p.button.removeAttribute('aria-busy');
    sync(p);
    if (ok) i18n.clear(p.status);
    else i18n.set(p.status, 'js.lang.failed');
  }

  function listen(p) {
    p.button.addEventListener('click', function () {
      if (isOpen(p)) close(p, false);
      else open(p);
    });
    p.button.addEventListener('keydown', function (ev) { list.onButtonKey(p, ev); });
    p.menu.addEventListener('keydown', function (ev) { list.onMenuKey(p, ev); });
    p.menu.addEventListener('click', function (ev) {
      const o = ev.target.closest('.lang-option');
      if (o) return choose(p, o.dataset.lang);
    });
    p.wrap.addEventListener('focusout', function (ev) {
      if (ev.relatedTarget && !p.wrap.contains(ev.relatedTarget)) close(p, false);
    });
    document.addEventListener('click', function (ev) {
      if (!p.wrap.contains(ev.target)) close(p, false);
    });
    i18n.onChange(function () { sync(p); });
  }

  // wire shows the picker and handles choices. Safe to call more than once.
  function wire() {
    const button = document.getElementById('lang-button');
    const menu = document.getElementById('lang-menu');
    const wrap = button ? button.closest('.lang-pick') : null;
    if (!wrap || !menu || button.dataset.wired || i18n.locales().length < 2) return;
    button.dataset.wired = '1';
    const p = { button, menu, wrap, code: wrap.querySelector('.lang-code'), status: document.getElementById('lang-status') };
    sync(p);
    listen(p);
    wrap.hidden = false;
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', wire);
  else wire();

  window.goneLangPicker = Object.freeze({ wire: wire });
})();
