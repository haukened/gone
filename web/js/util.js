'use strict';

// Shared helpers for gone page scripts: timing logs, delays, element
// building, and copy feedback. No helper here parses HTML.
// Exposed as window.goneUtil.
(function utilModule() {
  if (window.goneUtil) return;

  const COPIED_MS = 2200;
  const COPY_FAILED = 'Couldn\u2019t copy automatically. It\u2019s selected: press Ctrl+C (\u2318C on Mac).';
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

  // flashCopied relabels btn's <span> to "Copied" and announces message in
  // the status live region, restoring both after a moment. btn stays enabled
  // so keyboard focus is never dropped.
  function flashCopied(btn, status, message) {
    const label = btn.querySelector('span');
    const prev = copyTimers.get(btn);
    if (prev) clearTimeout(prev.timer);
    const idle = prev ? prev.idle : label.textContent;
    label.textContent = 'Copied';
    setText(status, message);
    const timer = setTimeout(function () {
      label.textContent = idle;
      setText(status, '');
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
      setText(status, COPY_FAILED);
      return false;
    }
  }

  window.goneUtil = Object.freeze({
    logTiming: logTiming,
    sleep: sleep,
    allPresent: allPresent,
    setText: setText,
    el: el,
    flashCopied: flashCopied,
    copyText: copyText
  });
})();
