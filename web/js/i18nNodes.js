'use strict';

// Node helpers for goneI18n: show a message key, attribute, formatted value
// or plain text on a node and remember it, so a language change re-renders
// it. Loaded before i18n.js. Exposed as window.goneI18nNodes.
(function i18nNodesModule() {
  if (window.goneI18nNodes) return;

  // create returns the node helpers for ctx: t, format, readArgs and
  // renderNode.
  function create(ctx) {
    const t = ctx.t;

    function setArgs(node, args) {
      if (args) node.setAttribute('data-i18n-args', JSON.stringify(args));
      else node.removeAttribute('data-i18n-args');
    }

    // set shows key on node and remembers it, so a language change re-renders it.
    function set(node, key, args) {
      if (!node) return;
      node.setAttribute('data-i18n', key);
      setArgs(node, args);
      ctx.renderNode(node);
    }

    // setAttr shows key in node's attribute attr and remembers it.
    function setAttr(node, attr, key, args) {
      if (!node) return;
      const pairs = (node.getAttribute('data-i18n-attr') || '').split(';').filter(function (p) {
        return p && p.indexOf(attr + ':') !== 0;
      });
      pairs.push(attr + ':' + key);
      node.setAttribute('data-i18n-attr', pairs.join(';'));
      if (args) setArgs(node, args);
      node.setAttribute(attr, t(key, args));
    }

    // value shows a formatted value (number, size, duration, date) on node and
    // remembers it, so a language change re-formats it.
    function value(node, v) {
      if (!node) return;
      node.setAttribute('data-i18n-value', JSON.stringify(v));
      node.textContent = ctx.format(v);
    }

    // plain shows text that is not translated (such as a name the visitor
    // typed) and forgets any message key on node.
    function plain(node, text) {
      if (!node) return;
      node.removeAttribute('data-i18n');
      node.removeAttribute('data-i18n-args');
      node.removeAttribute('data-i18n-value');
      node.textContent = text;
    }

    // snapshot remembers what node shows (its key and args, or its plain text)
    // so restore can put it back later, in whatever language is current then.
    function snapshot(node) {
      if (!node) return null;
      return { key: node.getAttribute('data-i18n'), args: ctx.readArgs(node), text: node.textContent };
    }

    function restore(node, snap) {
      if (!node || !snap) return;
      if (snap.key) set(node, snap.key, snap.args);
      else plain(node, snap.text);
    }

    // clear empties node and forgets any message key.
    function clear(node) {
      plain(node, '');
    }

    // setTitle sets the document title from key.
    function setTitle(key, args) {
      const el = document.querySelector('title');
      if (el) set(el, key, args);
      document.title = t(key, args);
    }

    return { set: set, setAttr: setAttr, value: value, plain: plain, clear: clear, snapshot: snapshot, restore: restore, setTitle: setTitle };
  }

  window.goneI18nNodes = Object.freeze({ create: create });
})();
