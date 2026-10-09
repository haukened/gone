'use strict';
/* global localStorage */

// Keeps a revealed message covered until the recipient chooses to show it, so
// it isn't on screen during a screen share. While covered, the text is not in
// the page at all; Copy still works. "Always show" is remembered per device.
(function consumeViewMaskModule() {
  if (window.goneConsumeViewMask || !window.goneConsumeViewBase) return;
  const ctx = window.goneConsumeViewBase;
  const dom = ctx.dom;
  const STORAGE_KEY = 'goneAlwaysShow';
  const ROWS = ['•'.repeat(22), '•'.repeat(14), '•'.repeat(18), '•'.repeat(9) + ' ' + '•'.repeat(8), '•'.repeat(12)];
  let text = '';

  function remembered() {
    try {
      return localStorage.getItem(STORAGE_KEY) === '1';
    } catch {
      return false;
    }
  }

  function remember(on) {
    try {
      if (on) localStorage.setItem(STORAGE_KEY, '1');
      else localStorage.removeItem(STORAGE_KEY);
    } catch {
      // Only a convenience: without storage the message starts covered.
    }
  }

  function lineCount(t) {
    return t.replace(/\s+$/, '').split('\n').length;
  }

  // sizeOf describes the message without showing it, as a message key and
  // args: lines, or characters for a one-liner such as a password.
  function sizeOf(t) {
    const lines = lineCount(t);
    if (lines > 1) return { key: 'js.consume.lines', args: { count: lines } };
    return { key: 'js.consume.characters', args: { count: Array.from(t).length } };
  }

  function setShown(shown) {
    dom.output.textContent = shown ? text : '';
    dom.output.hidden = !shown;
    dom.cover.hidden = shown;
    if (dom.messagePanel) dom.messagePanel.classList.toggle('is-shown', shown);
    if (dom.showLabel) ctx.i18n.set(dom.showLabel, shown ? 'js.common.hide' : 'js.common.show');
  }

  function maskable() {
    return Boolean(dom.output && dom.cover && dom.show);
  }

  // apply puts text in the message panel, covered unless the recipient chose
  // to always show secrets on this device.
  function apply(t) {
    text = t;
    if (!maskable()) {
      if (dom.output) dom.output.textContent = t;
      return;
    }
    const size = sizeOf(t);
    ctx.i18n.set(dom.size, size.key, size.args);
    if (dom.coverDots) dom.coverDots.textContent = ROWS.slice(0, Math.min(lineCount(t), ROWS.length)).join('\n');
    const always = remembered();
    if (dom.alwaysShow) dom.alwaysShow.checked = always;
    setShown(always);
  }

  function show() {
    if (maskable()) setShown(true);
  }

  if (maskable()) {
    dom.show.addEventListener('click', function () { setShown(dom.output.hidden); });
    if (dom.alwaysShow) {
      dom.alwaysShow.addEventListener('change', function () {
        remember(dom.alwaysShow.checked);
        if (dom.alwaysShow.checked) setShown(true);
      });
    }
  }

  window.goneConsumeViewMask = Object.freeze({ apply: apply, show: show, sizeOf: sizeOf });
})();
