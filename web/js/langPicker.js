'use strict';

// Language picker: the globe pill beside the theme toggle. The pill is a
// disclosure button for a short list of buttons, one per language, each in
// its own name and marked with lang. The current language carries
// aria-current. Choosing one switches the page in place through
// window.goneI18n; if that language can't be loaded, the page stays as it was
// and the failure is announced. Arrow keys, Home and End move through the
// list; Escape closes it and returns to the pill; clicking or tabbing away
// closes it. Shown only when scripts run and more than one language is
// available. Exposed as window.goneLangPicker.
(function langPickerModule() {
  if (window.goneLangPicker || !window.goneI18n) return;
  const i18n = window.goneI18n;

  // codeFor returns the short code shown on the pill, e.g. "PT" for pt-BR.
  function codeFor(tag) {
    return tag.split('-')[0].toUpperCase();
  }

  function options(p) {
    return Array.from(p.menu.querySelectorAll('.lang-option'));
  }

  function sync(p) {
    if (p.code) p.code.textContent = codeFor(i18n.locale);
    options(p).forEach(function (o) {
      if (o.dataset.lang === i18n.locale) o.setAttribute('aria-current', 'true');
      else o.removeAttribute('aria-current');
    });
  }

  function isOpen(p) {
    return !p.menu.hidden;
  }

  // open shows the list and focuses the current language.
  function open(p) {
    p.menu.hidden = false;
    p.button.setAttribute('aria-expanded', 'true');
    const all = options(p);
    const current = all.find(function (o) { return o.dataset.lang === i18n.locale; });
    (current || all[0]).focus();
  }

  // close hides the list, returning focus to the pill when asked.
  function close(p, refocus) {
    if (!isOpen(p)) return;
    p.menu.hidden = true;
    p.button.setAttribute('aria-expanded', 'false');
    if (refocus) p.button.focus();
  }

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

  // step returns the option index a navigation key moves to, or -1.
  function step(key, at, count) {
    const moves = { ArrowDown: (at + 1) % count, ArrowUp: (at - 1 + count) % count, Home: 0, End: count - 1 };
    return key in moves ? moves[key] : -1;
  }

  function onMenuKey(p, ev) {
    if (ev.key === 'Escape') {
      ev.preventDefault();
      close(p, true);
      return;
    }
    const all = options(p);
    const next = step(ev.key, all.indexOf(document.activeElement), all.length);
    if (next < 0) return;
    ev.preventDefault();
    all[next].focus();
  }

  function onButtonKey(p, ev) {
    if (ev.key !== 'ArrowDown' && ev.key !== 'ArrowUp') return;
    ev.preventDefault();
    open(p);
  }

  function listen(p) {
    p.button.addEventListener('click', function () {
      if (isOpen(p)) close(p, false);
      else open(p);
    });
    p.button.addEventListener('keydown', function (ev) { onButtonKey(p, ev); });
    p.menu.addEventListener('keydown', function (ev) { onMenuKey(p, ev); });
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
