// Docs pages: marks the section being read in "On this page", keeps the
// narrow-screen drop-downs tidy (one open at a time, closed on a choice, on
// Escape or a tap outside), scrolls the sidebar to the current page, and
// marks wide code and tables so their scrolling edge fades.
// Every page works without it.
(function () {
  'use strict';

  // A heading counts as "being read" once its top passes this line.
  const READ_LINE = 0.3;

  // tocLinks maps each heading id to the "On this page" links for it (the
  // list beside the text and the one in the narrow-screen bar).
  function tocLinks() {
    const byId = new Map();
    document.querySelectorAll('.docs-toc a[href^="#"]').forEach(function (a) {
      const id = decodeURIComponent(a.hash.slice(1));
      if (!byId.has(id)) byId.set(id, []);
      byId.get(id).push(a);
    });
    return byId;
  }

  // currentHeading returns the last heading above the reading line.
  function currentHeading(heads) {
    const line = window.innerHeight * READ_LINE;
    let current = heads[0];
    for (const h of heads) {
      if (h.getBoundingClientRect().top > line) break;
      current = h;
    }
    return current;
  }

  function scrollSpy() {
    const byId = tocLinks();
    const heads = [...byId.keys()].map((id) => document.getElementById(id)).filter(Boolean);
    if (!heads.length) return;
    let marked = null;
    let queued = false;
    function update() {
      queued = false;
      const h = currentHeading(heads);
      if (h === marked) return;
      if (marked) byId.get(marked.id).forEach((a) => a.removeAttribute('aria-current'));
      byId.get(h.id).forEach((a) => a.setAttribute('aria-current', 'true'));
      document.querySelectorAll('[data-toc-current]').forEach((s) => { s.textContent = h.textContent.trim(); });
      marked = h;
    }
    window.addEventListener('scroll', function () {
      if (!queued) { queued = true; window.requestAnimationFrame(update); }
    }, { passive: true });
    update();
  }

  function closeDrops(except) {
    document.querySelectorAll('.docs-drop[open]').forEach(function (d) {
      if (d !== except) d.open = false;
    });
  }

  function dropDowns() {
    const drops = document.querySelectorAll('.docs-drop');
    drops.forEach(function (d) {
      // Browsers without <details name> still get one open at a time.
      d.addEventListener('toggle', function () { if (d.open) closeDrops(d); });
      d.querySelector('.docs-drop-panel').addEventListener('click', function (e) {
        if (e.target.closest('a')) d.open = false;
      });
    });
    document.addEventListener('keydown', function (e) {
      const open = document.querySelector('.docs-drop[open]');
      if (e.key !== 'Escape' || !open) return;
      open.open = false;
      open.querySelector('summary').focus();
    });
    document.addEventListener('click', function (e) {
      if (!e.target.closest('.docs-drop')) closeDrops(null);
    });
  }

  // reveal scrolls a list's scroller so its current page is in view,
  // without moving the page itself.
  function reveal(scroller) {
    const a = scroller && scroller.querySelector('a[aria-current="page"]');
    if (!a) return;
    const box = scroller.getBoundingClientRect();
    const at = a.getBoundingClientRect();
    if (at.top < box.top || at.bottom > box.bottom) {
      scroller.scrollTop += at.top - box.top - box.height / 3;
    }
  }

  function sidebar() {
    reveal(document.querySelector('.docs-side'));
    const menu = document.querySelector('.docs-drop-menu');
    if (menu) {
      menu.addEventListener('toggle', function () {
        if (menu.open) reveal(menu.querySelector('.docs-drop-panel'));
      });
    }
  }

  // scrollCue marks a box that scrolls sideways with where it is scrolled
  // to, so CSS can fade the edge that has more to see.
  function scrollCue(box) {
    function mark() {
      const room = box.scrollWidth - box.clientWidth;
      if (room <= 1) { box.removeAttribute('data-scroll'); return; }
      const x = box.scrollLeft;
      box.dataset.scroll = x <= 1 ? 'start' : x >= room - 1 ? 'end' : 'middle';
    }
    box.addEventListener('scroll', mark, { passive: true });
    window.addEventListener('resize', mark);
    mark();
  }

  // copyName says what a code block's copy button copies and where.
  function copyName(block, heading) {
    const label = block.querySelector('.codeblock-label');
    let what = 'code';
    if (block.classList.contains('codeblock-terminal')) what = 'commands';
    else if (label) what = label.textContent.trim();
    return 'Copy ' + what + ' from ' + heading.textContent.trim();
  }

  // nameCopyButtons names each copy button after what it copies and the
  // section it's in ("Copy commands from Start it"), numbering repeats, so a
  // screen reader can tell them apart. A named file keeps its own name.
  function nameCopyButtons() {
    const seen = new Map();
    let heading = document.querySelector('.docs-page h1');
    document.querySelectorAll('.docs-prose :is(h2, h3, h4), .docs-prose .codeblock-copy').forEach(function (el) {
      if (!el.classList.contains('codeblock-copy')) { heading = el; return; }
      const block = el.closest('.codeblock');
      if (!heading || block.classList.contains('codeblock-file')) return;
      const name = copyName(block, heading);
      const n = (seen.get(name) || 0) + 1;
      seen.set(name, n);
      el.setAttribute('aria-label', n > 1 ? name + ' (' + n + ')' : name);
    });
  }

  function init() {
    document.querySelectorAll('.codeblock pre, .table-wrap').forEach(scrollCue);
    nameCopyButtons();
    scrollSpy();
    dropDowns();
    sidebar();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
