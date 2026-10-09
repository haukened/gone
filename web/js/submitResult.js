'use strict';

// Result view shown after a secret is created: fills the server-rendered
// #result view with the share link and expiry, wires the copy button, and
// swaps it in for #compose. Requires window.goneUtil and window.goneI18n; uses
// window.goneResultQr for the opt-in QR code when it is loaded. Exposed as
// window.goneResultPanel.
(function resultPanelModule() {
  if (window.goneResultPanel || !window.goneUtil || !window.goneI18n) return;
  const util = window.goneUtil;

  function byId(id) {
    return document.getElementById(id);
  }

  function selectAll(input) {
    input.focus();
    input.select();
  }

  // setExpiry writes a human expiry and the machine-readable datetime.
  function setExpiry(node, expiresAt) {
    if (!node) return;
    const when = new Date(expiresAt);
    window.goneI18n.value(node, { date: when.toISOString(), style: 'datetime' });
    node.setAttribute('datetime', when.toISOString());
  }

  function wireCopy(btn, input, status, message) {
    btn.addEventListener('click', async function () {
      const ok = await util.copyText(input.value, function () { selectAll(input); }, status);
      if (ok) util.flashCopied(btn, status, message);
    });
  }

  // wireOnce attaches the copy handler the first time a button is shown.
  function wireOnce(btn, input, status, message) {
    if (btn.dataset.wired) return;
    btn.dataset.wired = '1';
    wireCopy(btn, input, status, message);
  }

  // showManage fills the collapsed "Manage this secret" section, or hides it
  // when there is no manage link (for example an older server).
  function showManage(url) {
    const section = byId('manage-disclosure');
    const input = byId('manage-link');
    const btn = byId('copy-manage');
    if (!util.allPresent([section, input, btn])) return;
    section.open = false;
    if (!url) {
      section.hidden = true;
      input.value = '';
      return;
    }
    input.value = url;
    wireOnce(btn, input, byId('manage-copy-status'), 'js.result.manageCopied');
    section.hidden = false;
  }

  // showQr offers the opt-in QR code for the share link, closed. The button
  // stays hidden when the QR module or its markup is missing.
  function showQr(input) {
    const el = { button: byId('qr-toggle'), figure: byId('share-qr'), code: byId('share-qr-code'), input: input };
    if (!window.goneResultQr || !util.allPresent([el.button, el.figure, el.code])) return;
    window.goneResultQr.attach(el);
  }

  // show fills and reveals the result view, hiding the compose view.
  //
  // opts: {shareURL, manageURL = '', expiresAt, passphrase = false, focus = true}.
  // Returns the result view, or null when the page lacks it.
  function show(opts) {
    const view = byId('result');
    const input = byId('share-link');
    const btn = byId('copy-link');
    if (!util.allPresent([view, input, btn])) return null;
    input.value = opts.shareURL;
    setExpiry(byId('result-expiry'), opts.expiresAt);
    wireOnce(btn, input, byId('copy-status'), 'js.result.linkCopied');
    showQr(input);
    showManage(opts.manageURL || '');
    const passNote = byId('result-pass-note');
    if (passNote) passNote.hidden = !opts.passphrase;
    const compose = byId('compose');
    if (compose) compose.hidden = true;
    view.hidden = false;
    window.goneI18n.setTitle('js.result.title');
    const heading = byId('result-heading');
    if (opts.focus !== false && heading) heading.focus();
    return view;
  }

  window.goneResultPanel = Object.freeze({ show: show });
})();
