'use strict';

// Result view shown after a secret is created: fills the server-rendered
// #result view with the share link and expiry, wires the copy button, and
// swaps it in for #compose. Requires window.goneUtil. Exposed as
// window.goneResultPanel.
(function resultPanelModule() {
  if (window.goneResultPanel || !window.goneUtil) return;
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
    node.textContent = when.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
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
    wireOnce(btn, input, byId('manage-copy-status'), 'Manage link copied to clipboard.');
    section.hidden = false;
  }

  // show fills and reveals the result view, hiding the compose view.
  //
  // opts: {shareURL, manageURL = '', expiresAt, focus = true}. Returns the
  // result view, or null when the page lacks it.
  function show(opts) {
    const view = byId('result');
    const input = byId('share-link');
    const btn = byId('copy-link');
    if (!util.allPresent([view, input, btn])) return null;
    input.value = opts.shareURL;
    setExpiry(byId('result-expiry'), opts.expiresAt);
    wireOnce(btn, input, byId('copy-status'), 'Link copied to clipboard.');
    showManage(opts.manageURL || '');
    const compose = byId('compose');
    if (compose) compose.hidden = true;
    view.hidden = false;
    document.title = 'Gone \u00b7 Your link is ready';
    const heading = byId('result-heading');
    if (opts.focus !== false && heading) heading.focus();
    return view;
  }

  window.goneResultPanel = Object.freeze({ show: show });
})();
