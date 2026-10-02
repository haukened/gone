'use strict';

// Result panel shown after a secret is created: the share link, its expiry,
// a copy button, and a link back to the form. Requires window.goneUtil.
// Exposed as window.goneResultPanel.
(function resultPanelModule() {
  if (window.goneResultPanel || !window.goneUtil) return;
  const util = window.goneUtil;
  const el = util.el;

  // Static, trusted icon markup (never user or server data).
  const BACK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-left-icon lucide-arrow-left"><path d="m12 19-7-7 7-7"/><path d="M19 12H5"/></svg>';
  const COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';
  const CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="1.1em" height="1.1em" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
  const WARN_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4"/><path d="M12 17h.01"/><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"/></svg>';
  const COPY_LABEL = 'Copy Link ' + COPY_ICON;

  function warningCard() {
    const warn = el('p', { className: 'security-warning-card' });
    util.setStaticHTML(warn, WARN_ICON);
    warn.appendChild(el('span', { textContent: 'Anyone with this link can view the secret exactly once.' }));
    return warn;
  }

  function expiryHint(expiresAt) {
    const time = el('time', { textContent: new Date(expiresAt).toLocaleString() });
    time.setAttribute('datetime', expiresAt);
    return el('p', { className: 'hint' }, [el('span', {}, [document.createTextNode('Expires at '), time])]);
  }

  function actionsRow(shareURL, input) {
    const back = el('a', { href: '/', className: 'back-link' });
    util.setStaticHTML(back, BACK_ICON + ' Create Another');
    const copy = el('button', { type: 'button', className: 'copy-primary-btn' });
    copy.setAttribute('aria-label', 'Copy full share link');
    util.setStaticHTML(copy, COPY_LABEL);
    copy.addEventListener('click', async function () {
      const ok = await util.copyText(shareURL, function () { input.focus(); input.select(); });
      if (ok) util.flashCopied(copy, COPY_LABEL, 'Copied! ' + CHECK_ICON);
      return ok;
    });
    return el('div', { className: 'result-actions' }, [back, copy]);
  }

  function buildPanel(shareURL, expiresAt) {
    const input = el('input', { className: 'share-link', id: 'share-link', type: 'text', readOnly: true, value: shareURL });
    const card = el('div', { className: 'card' }, [expiryHint(expiresAt), input, actionsRow(shareURL, input)]);
    const outer = el('div', { id: 'result-outer' }, [
      el('h2', { className: 'underline', textContent: 'Share This Link' }),
      warningCard(),
      card
    ]);
    return { panel: el('div', {}, [outer]), input: input };
  }

  // focusStart focuses the link and keeps its beginning visible; some
  // browsers scroll a long input to the end on focus.
  function focusStart(input) {
    input.focus();
    try {
      requestAnimationFrame(function () {
        input.selectionStart = 0;
        input.selectionEnd = 0;
        input.scrollLeft = 0;
      });
    } catch (_) { /* non-critical */ }
  }

  // show renders the panel in place of replaceTarget (or appends it to body).
  //
  // opts: {shareURL, expiresAt, replaceTarget, focus = true}.
  function show(opts) {
    const built = buildPanel(opts.shareURL, opts.expiresAt);
    if (opts.replaceTarget) opts.replaceTarget.replaceWith(built.panel);
    else document.body.appendChild(built.panel);
    if (opts.focus !== false) focusStart(built.input);
    console.log('[gone] result panel shown');
    return built.panel;
  }

  window.goneResultPanel = Object.freeze({ show: show });
})();
