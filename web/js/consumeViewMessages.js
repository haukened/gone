'use strict';

// Message rendering and view switching for consumed secrets.
(function consumeViewMessagesModule() {
  if (window.goneConsumeViewMessages || !window.goneConsumeViewBase || !window.goneConsumeViewMask) return;
  const ctx = window.goneConsumeViewBase;
  const dom = ctx.dom;
  const mask = window.goneConsumeViewMask;
  const VIEWS = ['open', 'revealed', 'gone'];
  const TITLES = { revealed: 'Gone \u00b7 Here\u2019s your secret', gone: 'Gone \u00b7 This secret is gone' };

  function onOpen(handler) {
    if (dom.open) dom.open.addEventListener('click', handler);
  }

  function switchTo(name) {
    VIEWS.forEach(function (v) {
      const node = ctx.byId(`view-${v}`);
      if (node) node.hidden = v !== name;
    });
    if (TITLES[name]) document.title = TITLES[name];
    const heading = ctx.byId(`${name}-heading`);
    if (heading) heading.focus();
  }

  // selectOutput is the copy fallback: the recipient copies by hand, so the
  // message has to be shown first.
  function selectOutput() {
    mask.show();
    const range = document.createRange();
    range.selectNodeContents(dom.output);
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
  }

  function wireCopy(text) {
    const btn = dom.copy;
    if (!btn) return;
    btn.addEventListener('click', async function () {
      const ok = await ctx.util.copyText(text, selectOutput, dom.copyStatus);
      if (ok) ctx.util.flashCopied(btn, dom.copyStatus, 'Message copied to clipboard.');
    });
  }

  function showMessage(text) {
    if (!text || !dom.output) return;
    mask.apply(text);
    if (dom.messagePanel) dom.messagePanel.hidden = false;
    wireCopy(text);
  }

  window.goneConsumeViewMessages = Object.freeze({ onOpen: onOpen, switchTo: switchTo, showMessage: showMessage });
})();
