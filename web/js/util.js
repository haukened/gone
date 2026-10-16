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

  function viewId(v) {
    return `view-${v}`;
  }

  // showView shows the named view among views and hides the others, sets the
  // page title from titles (when it has one), and moves focus to the view's
  // heading so screen readers announce the change. idOf maps a view name to
  // its element id (default "view-<name>").
  function showView(views, name, titles, idOf) {
    const idFor = idOf || viewId;
    views.forEach(function (v) {
      const node = document.getElementById(idFor(v));
      if (node) node.hidden = v !== name;
    });
    if (titles[name]) window.goneI18n.setTitle(titles[name]);
    const heading = document.getElementById(`${name}-heading`);
    if (heading) heading.focus();
  }

  // TOKEN_RE matches a bearer token (manage, fill or claim): 32 bytes as
  // strict base64url, so 43 characters whose last one leaves its two unused
  // bits zero (docs/protocol.md sections 2.1 and 8). Anchored and
  // fixed-length, so it runs in linear time.
  const TOKEN_RE = /^[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]$/;

  // isToken reports whether t is a well-formed bearer token.
  function isToken(t) {
    return typeof t === 'string' && TOKEN_RE.test(t);
  }

  function replaceWithoutFragment() {
    try {
      history.replaceState(null, '', location.pathname + location.search);
    } catch (e) {
      console.warn('[gone] could not clear the address bar fragment', e);
    }
  }

  // clearFragment replaces the address-bar URL with the same path and query
  // but no fragment, so a link key or token that a page script has already
  // read does not linger in the address bar, history or session restore.
  // Call it after reading location.hash; the page keeps its in-memory copy.
  // While the page is still loading it waits for the load event, so the
  // same-document navigation never overlaps the load (WebKit's automation
  // can stall on that).
  function clearFragment() {
    if (document.readyState === 'complete') replaceWithoutFragment();
    else window.addEventListener('load', replaceWithoutFragment, { once: true });
  }

  window.goneUtil = Object.freeze({
    logTiming: logTiming,
    sleep: sleep,
    allPresent: allPresent,
    cryptoAvailable: cryptoAvailable,
    setText: setText,
    el: el,
    flashCopied: flashCopied,
    copyText: copyText,
    showView: showView,
    isToken: isToken,
    clearFragment: clearFragment
  });
})();
