// Copy buttons for commands: <button data-copy="ID"> copies the text of the
// element with that ID. Like the app's link box, the button reads "Copied"
// for a moment and a nearby role="status" element announces it; the button
// stays enabled so keyboard focus isn't lost. Buttons stay hidden where there
// is no clipboard, so the page never offers a button that can't work.
(function () {
  'use strict';

  var COPIED_MS = 2200;

  // statusFor returns the live region that announces a copy: the nearest
  // [data-copy-status] in the same section.
  function statusFor(btn) {
    var box = btn.closest('section, .pane, main');
    return box ? box.querySelector('[data-copy-status]') : null;
  }

  // flash shows "Copied" on the button and in the status, then restores both.
  function flash(btn, message) {
    var label = btn.querySelector('span');
    var status = statusFor(btn);
    var before = label ? label.textContent : '';
    if (label) label.textContent = message;
    if (status) status.textContent = message === 'Copied' ? 'Copied to the clipboard.' : message;
    window.setTimeout(function () {
      if (label) label.textContent = before;
      if (status) status.textContent = '';
    }, COPIED_MS);
  }

  function copy(btn) {
    var target = document.getElementById(btn.getAttribute('data-copy'));
    if (!target) return;
    navigator.clipboard.writeText(target.textContent.trim()).then(function () {
      flash(btn, 'Copied');
    }, function () {
      flash(btn, 'Copy failed: select the text instead');
    });
  }

  function init() {
    if (!navigator.clipboard || !window.isSecureContext) return;
    document.querySelectorAll('button[data-copy]').forEach(function (btn) {
      btn.hidden = false;
      btn.addEventListener('click', function () { copy(btn); });
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
