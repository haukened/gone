'use strict';

// Recipient passphrase field controls for v2 consume links.
(function consumeViewPassphraseModule() {
  if (!window.goneConsumeViewBase || window.goneConsumeViewBase.pass) return;
  const ctx = window.goneConsumeViewBase;
  const dom = ctx.dom;
  let needsPass = false;

  function missing() {
    return needsPass && !dom.pass.value;
  }

  function setShown(shown) {
    dom.pass.type = shown ? 'text' : 'password';
    ctx.util.setText(dom.passToggle ? dom.passToggle.querySelector('span') : null, shown ? 'Hide' : 'Show');
  }

  function onPassKey(ev) {
    if (ev.key !== 'Enter') return;
    ev.preventDefault();
    if (dom.open) dom.open.click();
  }

  function show() {
    if (!dom.pass) return false;
    needsPass = true;
    if (dom.passField) dom.passField.hidden = false;
    // The passphrase warning replaces the general hint: it says the same
    // and covers what a wrong passphrase does.
    if (dom.passWarn) {
      dom.passWarn.hidden = false;
      if (dom.openHint) dom.openHint.hidden = true;
      if (dom.open) dom.open.setAttribute('aria-describedby', 'open-pass-warn');
    }
    dom.pass.addEventListener('input', function () { dom.pass.setAttribute('aria-invalid', 'false'); ctx.syncOpen(); });
    dom.pass.addEventListener('keydown', onPassKey);
    if (dom.passToggle) dom.passToggle.addEventListener('click', function () { setShown(dom.pass.type === 'password'); });
    ctx.syncOpen();
    return true;
  }

  function failed() {
    ctx.state.openLabel = 'Try again';
    ctx.util.setText(dom.openLabel, ctx.state.openLabel);
    // The error now says the server copy is gone and this page holds the
    // only one, so the before-opening hint would only contradict it.
    if (dom.passWarn) dom.passWarn.hidden = true;
    if (dom.open) dom.open.setAttribute('aria-describedby', 'consume-error-text');
    if (!dom.pass) return;
    dom.pass.setAttribute('aria-invalid', 'true');
    dom.pass.focus();
    dom.pass.select();
  }

  function clear() {
    if (!dom.pass) return;
    dom.pass.value = '';
    setShown(false);
  }

  ctx.pass = { show: show, value: function () { return dom.pass ? dom.pass.value : ''; }, missing: missing, focus: function () { if (dom.pass) dom.pass.focus(); }, failed: failed, clear: clear };
})();
