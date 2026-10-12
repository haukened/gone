'use strict';

// The language picker's list: showing the current language, opening and
// closing the list, and moving through it with the keyboard. Each function
// takes the picker's elements p ({button, menu, wrap, code, status}).
// Exposed as window.goneLangPickerMenu.
(function langPickerMenuModule() {
  if (window.goneLangPickerMenu || !window.goneI18n) return;
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

  window.goneLangPickerMenu = Object.freeze({
    sync: sync,
    isOpen: isOpen,
    open: open,
    close: close,
    onMenuKey: onMenuKey,
    onButtonKey: onButtonKey
  });
})();
