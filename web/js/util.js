/* global localStorage */
'use strict';

// Shared helpers for gone page scripts: timing logs, delays, element
// building, and copy feedback. No helper here parses HTML. Copy feedback is
// translated through window.goneI18n. Exposed as window.goneUtil.
(function utilModule() {
  if (window.goneUtil) return;

  const COPIED_MS = 2200;
  const copyTimers = new WeakMap();

  // readDebugTiming enables [gone][timing] logs via ?debug=timing or the
  // goneDebugTiming=1 localStorage flag.
  function readDebugTiming() {
    try {
      if (new URLSearchParams(location.search).get('debug') === 'timing') return true;
      return Boolean(window.localStorage) && localStorage.getItem('goneDebugTiming') === '1';
    } catch {
      return false;
    }
  }

  const debugTiming = readDebugTiming();

  // logTiming logs the elapsed milliseconds for label when timing is enabled.
  function logTiming(label, begin, end) {
    if (!debugTiming) return;
    console.log(`[gone][timing] ${label}: ${(end - begin).toFixed(2)}ms`);
  }

  // sleep resolves after ms milliseconds.
  function sleep(ms) {
    return new Promise(function (resolve) { setTimeout(resolve, ms); });
  }

  // allPresent reports whether every value is truthy.
  function allPresent(values) {
    return values.every(Boolean);
  }

  // setText sets an optional element's text content.
  // cryptoAvailable reports whether WebCrypto can run here. Browsers only
  // expose crypto.subtle on secure (HTTPS or localhost) pages.
  function cryptoAvailable() {
    return typeof crypto !== 'undefined' && Boolean(crypto.subtle);
  }

  function setText(node, text) {
    if (node) node.textContent = text;
  }

  // el creates an element, assigns DOM properties, and appends children.
  // props must never include innerHTML/outerHTML; use textContent for text.
  function el(tag, props, children) {
    const node = document.createElement(tag);
    Object.assign(node, props);
    if (children) node.append(...children);
    return node;
  }

  // flashCopied relabels btn's <span> to "Copied" and announces the message
  // key in the status live region, restoring both after a moment. The label's
  // own message key is restored (so a language change meanwhile is kept), or
  // its text when it has none. btn stays enabled so keyboard focus is never
  // dropped.
  function flashCopied(btn, status, messageKey) {
    const i18n = window.goneI18n;
    const label = btn.querySelector('span');
    const prev = copyTimers.get(btn);
    if (prev) clearTimeout(prev.timer);
    const idle = prev ? prev.idle : i18n.snapshot(label);
    i18n.set(label, 'js.common.copied');
    if (status) i18n.set(status, messageKey);
    const timer = setTimeout(function () {
      i18n.restore(label, idle);
      if (status) i18n.clear(status);
      copyTimers.delete(btn);
    }, COPIED_MS);
    copyTimers.set(btn, { timer: timer, idle: idle });
  }

  // copyText writes text to the clipboard. On failure it runs select (if
  // given) so the user can copy by hand, and puts guidance in status.
  //
  // Returns a promise resolving to whether the copy succeeded.
  async function copyText(text, select, status) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      if (select) select();
      if (status) window.goneI18n.set(status, 'js.common.copyFailed');
      return false;
    }
  }

  window.goneUtil = Object.freeze({
    logTiming: logTiming,
    sleep: sleep,
    allPresent: allPresent,
    cryptoAvailable: cryptoAvailable,
    setText: setText,
    el: el,
    flashCopied: flashCopied,
    copyText: copyText
  });
})();
