'use strict';

// Inline confirm used by the manage and request pages: a start button that
// swaps for a confirm panel, focus moves for screen readers, and Escape to
// cancel. Elements are looked up by id when used; any may be missing.
// Exposed as window.goneInlineConfirm.
(function inlineConfirmModule() {
  if (window.goneInlineConfirm) return;

  function byId(id) {
    return document.getElementById(id);
  }

  function show(id, visible) {
    const node = byId(id);
    if (node) node.hidden = !visible;
  }

  function focus(id) {
    const node = byId(id);
    if (node) node.focus();
  }

  // create wires one inline confirm. ids: {start, panel, text, button}, where
  // start wraps the button that opens the panel, text is the panel's prompt
  // and button gets focus back when the panel closes.
  // Returns {open, close, onEscape(cancel)}.
  function create(ids) {
    return {
      open: function () {
        show(ids.start, false);
        show(ids.panel, true);
        focus(ids.text);
      },
      close: function () {
        show(ids.panel, false);
        show(ids.start, true);
        focus(ids.button);
      },
      onEscape: function (cancel) {
        const panel = byId(ids.panel);
        if (panel) panel.addEventListener('keydown', function (ev) { if (ev.key === 'Escape') cancel(); });
      }
    };
  }

  window.goneInlineConfirm = Object.freeze({ create: create });
})();
