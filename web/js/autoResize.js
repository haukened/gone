'use strict';

// Textarea auto-resize extracted from original app.js
(function autoResize() {
  const ta = document.getElementById('secret');
  if (!ta) return false;
  const max = 40 * 16; // 40rem assuming 16px root
  function grow() { ta.style.height = 'auto'; const next = Math.min(ta.scrollHeight, max); ta.style.height = next + 'px'; ta.style.overflowY = ta.scrollHeight > max ? 'auto' : 'hidden'; return true; }
  ta.addEventListener('input', grow); grow(); return true;
})();