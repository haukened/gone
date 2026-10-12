'use strict';

// DOM rendering for goneI18n: rewrites nodes marked with data-i18n,
// data-i18n-attr, data-i18n-value and data-i18n-hidden-in in the current
// language. Loaded before i18n.js. Exposed as window.goneI18nRender.
(function i18nRenderModule() {
  if (window.goneI18nRender) return;

  // readArgs returns a node's data-i18n-args, or undefined.
  function readArgs(node) {
    const raw = node.getAttribute('data-i18n-args');
    try {
      return raw ? JSON.parse(raw) : undefined;
    } catch {
      return undefined;
    }
  }

  // fillSlot sets a slot's words, or returns the words alone when the slot
  // element is missing.
  function fillSlot(el, text) {
    if (!el) return document.createTextNode(text);
    if (text !== '') el.textContent = text;
    return el;
  }

  // create returns the renderers for ctx: locale(), t, format, partText,
  // patternFor and tokens.
  function create(ctx) {
    // renderNode rewrites a data-i18n node's text. Rich-text slots keep their
    // elements (and attributes); only the words in and around them change. A
    // slot left empty in the message keeps its content, such as a <time> a
    // script fills in. An unknown key shows as itself, so it is easy to spot.
    function renderNode(node) {
      const key = node.getAttribute('data-i18n');
      const args = readArgs(node);
      const pattern = ctx.patternFor(key, args);
      const parts = pattern == null ? [] : ctx.tokens(pattern);
      if (!parts.some(function (p) { return p.kind === 'open'; })) {
        node.textContent = ctx.t(key, args);
        return;
      }
      node.replaceChildren.apply(node, richNodes(node, parts, args));
    }

    // richNodes builds a rich message's child nodes, reusing node's slots.
    function richNodes(node, parts, args) {
      const slots = {};
      node.querySelectorAll('[data-i18n-slot]').forEach(function (el) { slots[el.getAttribute('data-i18n-slot')] = el; });
      const out = [];
      let open = null;
      parts.forEach(function (p) {
        if (p.kind === 'open') {
          open = { el: slots[p.name] || null, text: '' };
        } else if (p.kind === 'close') {
          out.push(fillSlot(open.el, open.text));
          open = null;
        } else if (open) {
          open.text += ctx.partText(p, args);
        } else {
          out.push(document.createTextNode(ctx.partText(p, args)));
        }
      });
      return out;
    }

    // renderAttrs rewrites attributes listed as "attr:key;attr:key".
    function renderAttrs(node) {
      const args = readArgs(node);
      node.getAttribute('data-i18n-attr').split(';').forEach(function (pair) {
        const i = pair.indexOf(':');
        if (i > 0) node.setAttribute(pair.slice(0, i), ctx.t(pair.slice(i + 1), args));
      });
    }

    function renderValue(node) {
      try {
        node.textContent = ctx.format(JSON.parse(node.getAttribute('data-i18n-value')));
      } catch {
        // leave malformed values as rendered
      }
    }

    function renderHidden(node) {
      node.hidden = node.getAttribute('data-i18n-hidden-in').split(' ').indexOf(ctx.locale()) >= 0;
    }

    // apply re-renders every translated node under root (default: the page).
    function apply(root) {
      const scope = root || document;
      scope.querySelectorAll('[data-i18n]').forEach(renderNode);
      scope.querySelectorAll('[data-i18n-attr]').forEach(renderAttrs);
      scope.querySelectorAll('[data-i18n-value]').forEach(renderValue);
      scope.querySelectorAll('[data-i18n-hidden-in]').forEach(renderHidden);
    }

    return { readArgs: readArgs, renderNode: renderNode, apply: apply };
  }

  window.goneI18nRender = Object.freeze({ create: create });
})();
