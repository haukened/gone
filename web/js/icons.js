'use strict';

// Decorative icons that reference the server-rendered SVG sprite
// (<symbol id="i-NAME">) via <use>, built with DOM APIs so no markup string is
// ever parsed. Exposed as window.goneIcons.
(function iconsModule() {
  const SVG_NS = 'http://www.w3.org/2000/svg';
  const NAME_RE = /^[a-z]+$/;

  // make returns a new aria-hidden <svg class="ico"> that uses the sprite
  // symbol "i-<name>". name must be lowercase letters only; anything else
  // throws so a caller can never inject an arbitrary href.
  function make(name) {
    if (typeof name !== 'string' || !NAME_RE.test(name)) throw new Error(`invalid icon: ${name}`);
    const svg = document.createElementNS(SVG_NS, 'svg');
    svg.setAttribute('class', 'ico');
    svg.setAttribute('aria-hidden', 'true');
    svg.setAttribute('focusable', 'false');
    const use = document.createElementNS(SVG_NS, 'use');
    use.setAttribute('href', `#i-${name}`);
    svg.appendChild(use);
    return svg;
  }

  if (!window.goneIcons) window.goneIcons = Object.freeze({ make: make });
})();
