'use strict';

// Inline SVG icons built with DOM APIs, so no markup string is ever parsed
// (no HTML strings). Shape data is static and bundled with the app.
// Exposed as window.goneIcons.
(function iconsModule() {
  const SVG_NS = 'http://www.w3.org/2000/svg';
  const BASE_ATTRS = Object.freeze({
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    'stroke-width': '2',
    'stroke-linecap': 'round',
    'stroke-linejoin': 'round',
    'aria-hidden': 'true',
    focusable: 'false'
  });
  const SHAPES = new Map([
    ['copy', [
      ['rect', { width: '14', height: '14', x: '8', y: '8', rx: '2', ry: '2' }],
      ['path', { d: 'M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2' }]
    ]],
    ['check', [['path', { d: 'M20 6 9 17l-5-5' }]]],
    ['back', [['path', { d: 'm12 19-7-7 7-7' }], ['path', { d: 'M19 12H5' }]]],
    ['warn', [
      ['path', { d: 'M12 9v4' }],
      ['path', { d: 'M12 17h.01' }],
      ['path', { d: 'M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z' }]
    ]]
  ]);

  // svgNode creates an SVG element and sets each attribute in attrs.
  function svgNode(tag, attrs) {
    const node = document.createElementNS(SVG_NS, tag);
    Object.keys(attrs).forEach(function (k) { node.setAttribute(k, attrs[k]); });
    return node;
  }

  // make returns a new decorative <svg> for the named icon.
  //
  // name: one of copy, check, back, warn. size: a CSS length for width and
  // height (default '1em'). Throws on an unknown name.
  function make(name, size) {
    const shapes = SHAPES.get(name);
    if (!shapes) throw new Error(`unknown icon: ${name}`);
    const s = size || '1em';
    const svg = svgNode('svg', Object.assign({ width: s, height: s }, BASE_ATTRS));
    shapes.forEach(function (shape) { svg.appendChild(svgNode(shape[0], shape[1])); });
    return svg;
  }

  if (!window.goneIcons) window.goneIcons = Object.freeze({ make: make });
})();
