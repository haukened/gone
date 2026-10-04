'use strict';

// Message rendering and view switching for consumed secrets.
(function consumeViewMessagesModule() {
  if (window.goneConsumeViewMessages || !window.goneConsumeViewBase) return;
  const ctx = window.goneConsumeViewBase;
  const dom = ctx.dom;
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

  function selectOutput() {
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
    dom.output.textContent = text;
    if (dom.messagePanel) dom.messagePanel.hidden = false;
    wireCopy(text);
  }

  window.goneConsumeViewMessages = Object.freeze({ onOpen: onOpen, switchTo: switchTo, showMessage: showMessage });
})();
