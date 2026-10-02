'use strict';

// Result panel shown after a secret is created: the share link, its expiry,
// a copy button, and a link back to the form. Requires window.goneUtil and
// window.goneIcons. Exposed as window.goneResultPanel.
(function resultPanelModule() {
  const ICON_SIZE = '1.1em';
  const WARN_TEXT = 'Anyone with this link can view the secret exactly once.';

  function el(tag, props, children) {
    return window.goneUtil.el(tag, props, children);
  }

  function icon(name, size) {
    return window.goneIcons.make(name, size || ICON_SIZE);
  }

  function warningCard() {
    return el('p', { className: 'security-warning-card' }, [icon('warn', '18'), el('span', { textContent: WARN_TEXT })]);
  }

  function expiryHint(expiresAt) {
    const time = el('time', { textContent: new Date(expiresAt).toLocaleString() });
    time.setAttribute('datetime', expiresAt);
    return el('p', { className: 'hint' }, [el('span', {}, ['Expires at ', time])]);
  }

  function copyButton(shareURL, input) {
    const util = window.goneUtil;
    const copy = el('button', { type: 'button', className: 'copy-primary-btn' }, ['Copy Link ', icon('copy')]);
    copy.setAttribute('aria-label', 'Copy full share link');
    copy.addEventListener('click', async function () {
      const ok = await util.copyText(shareURL, function () { input.focus(); input.select(); });
      if (ok) util.flashCopied(copy, ['Copied! ', icon('check')]);
      return ok;
    });
    return copy;
  }

  function actionsRow(shareURL, input) {
    const back = el('a', { href: '/', className: 'back-link' }, [icon('back'), ' Create Another']);
    return el('div', { className: 'result-actions' }, [back, copyButton(shareURL, input)]);
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

  if (!window.goneResultPanel && window.goneUtil && window.goneIcons) {
    window.goneResultPanel = Object.freeze({ show: show });
  }
})();
