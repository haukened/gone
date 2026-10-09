'use strict';

// Opt-in QR code for the share link on the result view. The "QR" button in
// the link box is a disclosure: it shows a figure under the link with the full
// link, key included, drawn as an SVG. The code is always dark on white, in
// both themes, because many scanners can't read an inverted code. Only the
// send result uses it; request and reply links never get a QR code, because a
// code hides the link it opens. Requires window.goneQr. Exposed as
// window.goneResultQr.
(function resultQrModule() {
  if (window.goneResultQr || !window.goneQr) return;
  const SVG_NS = 'http://www.w3.org/2000/svg';
  const QUIET = 4;

  // pathData draws each row's dark runs as rectangles, offset by the quiet zone.
  function pathData(code) {
    const parts = [];
    for (let y = 0; y < code.size; y++) {
      for (let x = 0; x < code.size; x++) {
        if (!code.dark[y * code.size + x]) continue;
        let len = 1;
        while (x + len < code.size && code.dark[y * code.size + x + len]) len++;
        parts.push(`M${x + QUIET} ${y + QUIET}h${len}v1h-${len}z`);
        x += len;
      }
    }
    return parts.join('');
  }

  function svgNode(tag, attrs) {
    const node = document.createElementNS(SVG_NS, tag);
    Object.entries(attrs).forEach(([k, v]) => node.setAttribute(k, v));
    return node;
  }

  // svgFor returns an <svg> of text's QR code with a 4-module quiet zone.
  function svgFor(text) {
    const code = window.goneQr.encode(text);
    const n = code.size + QUIET * 2;
    const svg = svgNode('svg', { viewBox: `0 0 ${n} ${n}`, 'shape-rendering': 'crispEdges', 'aria-hidden': 'true', focusable: 'false' });
    svg.appendChild(svgNode('rect', { class: 'qr-light', width: n, height: n }));
    svg.appendChild(svgNode('path', { class: 'qr-dark', d: pathData(code) }));
    return svg;
  }

  function setOpen(el, open) {
    el.button.setAttribute('aria-expanded', open ? 'true' : 'false');
    el.figure.hidden = !open;
    if (!open) el.code.replaceChildren();
  }

  function toggle(el) {
    if (el.button.getAttribute('aria-expanded') === 'true') {
      setOpen(el, false);
      return;
    }
    try {
      el.code.replaceChildren(svgFor(el.input.value));
    } catch {
      el.button.hidden = true;
      return;
    }
    setOpen(el, true);
    if (typeof el.figure.scrollIntoView === 'function') el.figure.scrollIntoView({ block: 'nearest' });
  }

  // attach wires the button once, shows it, and starts closed. Call it each
  // time the result view shows a new link.
  // el: {button, figure, code, input}.
  function attach(el) {
    if (!el.button.dataset.wired) {
      el.button.dataset.wired = '1';
      el.button.addEventListener('click', function () { toggle(el); });
    }
    setOpen(el, false);
    el.button.hidden = false;
  }

  window.goneResultQr = Object.freeze({ attach: attach, svgFor: svgFor });
})();
