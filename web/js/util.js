'use strict';

// Shared helpers for gone page scripts: timing logs, delays, element
// building, and copy-button feedback. Exposed as window.goneUtil.
(function utilModule() {
  if (window.goneUtil) return;

  const COPIED_MS = 2200;
  const COPY_FAILED = 'Copy failed. Please press \u2318/Ctrl+C to copy manually.';

  // readDebugTiming enables [gone][timing] logs via ?debug=timing or the
  // goneDebugTiming=1 localStorage flag.
  function readDebugTiming() {
    try {
      if (new URLSearchParams(location.search).get('debug') === 'timing') return true;
      return Boolean(window.localStorage) && localStorage.getItem('goneDebugTiming') === '1';
    } catch (_) {
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

  // setStaticHTML replaces btn's content with html. Trust boundary: html must
  // be static markup bundled with the app (icons and fixed labels), never
  // user or server data.
  function setStaticHTML(btn, html) {
    btn.innerHTML = html;
  }

  // flashCopied shows doneHTML on btn briefly, then restores idleHTML. Both
  // must be static, trusted markup.
  function flashCopied(btn, idleHTML, doneHTML) {
    setStaticHTML(btn, doneHTML);
    btn.classList.add('copied');
    btn.disabled = true;
    setTimeout(function () {
      setStaticHTML(btn, idleHTML);
      btn.classList.remove('copied');
      btn.disabled = false;
    }, COPIED_MS);
  }

  // copyText writes text to the clipboard. On failure it runs onFail (if
  // given), then asks the user to copy manually.
  //
  // Returns a promise resolving to whether the copy succeeded.
  async function copyText(text, onFail) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (_) {
      if (onFail) onFail();
      alert(COPY_FAILED);
      return false;
    }
  }

  window.goneUtil = Object.freeze({
    logTiming: logTiming,
    sleep: sleep,
    allPresent: allPresent,
    setText: setText,
    el: el,
    setStaticHTML: setStaticHTML,
    flashCopied: flashCopied,
    copyText: copyText
  });
})();
