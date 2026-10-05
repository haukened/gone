'use strict';

const { reset, load, fastUtil, h } = require('./harness');

function optionalNode(omit, id, tag, props, kids) {
  const children = (kids || []).filter(Boolean);
  if (!omit.has(id)) return h(tag, Object.assign({ id }, props), children);
  return children.length ? h('div', {}, children) : null;
}

function openView(n, openLabel) {
  return n('view-open', 'div', {}, [
    n('open-heading', 'h1'),
    n('open-pass-field', 'div', { hidden: true }, [
      n('open-passphrase', 'input', { type: 'password' }),
      n('open-pass-toggle', 'button', {}, [h('span', { textContent: 'Show' })])
    ]),
    n('open-pass-warn', 'p', { hidden: true }),
    n('open-secret', 'button', {}, [h('svg'), openLabel]),
    n('download-progress', 'progress', { hidden: true }),
    n('consume-status', 'span'),
    n('consume-error', 'div', { hidden: true }, [n('consume-error-text', 'p')])
  ]);
}

// maskNodes are the Show and Hide buttons, cover and "always show" box, which only
// some tests include.
function maskNodes(n) {
  return [
    n('hide-secret', 'button', { hidden: true }),
    n('secret-cover', 'div', {}, [n('secret-cover-dots', 'pre'), n('secret-size', 'span'), n('show-secret', 'button')]),
    n('always-show', 'input', { type: 'checkbox' })
  ];
}

function revealedView(n, copyLabel, masked) {
  const panel = [n('copy-secret', 'button', {}, [h('svg'), copyLabel]), n('secret-output', 'div')].concat(masked ? maskNodes(n) : []);
  return n('view-revealed', 'div', { hidden: true }, [
    n('revealed-heading', 'h1'),
    n('ack-warning', 'div', { hidden: true }),
    n('message-panel', 'div', { hidden: true }, panel),
    n('copy-status', 'span'),
    n('file-section', 'div', { hidden: true }, [n('file-output-list', 'ul'), n('download-all', 'button', { hidden: true })])
  ]);
}

function page(env, skip, masked) {
  const omit = new Set(skip || []);
  const n = (id, tag, props, kids) => optionalNode(omit, id, tag, props, kids);
  const openLabel = h('span', { textContent: 'Open secret' });
  const copyLabel = h('span', { textContent: 'Copy message' });
  const views = [openView(n, openLabel), revealedView(n, copyLabel, masked), n('view-gone', 'div', { hidden: true }, [n('gone-heading', 'h1')])];
  env.document.body.append(...views.filter(Boolean));
  const $ = (id) => env.document.getElementById(id);
  return { $, openLabel, copyLabel };
}

function setup(t, skip, opts) {
  const o = opts || {};
  const env = reset('https://gone.test/secret/x', o.env);
  const dom = page(env, skip, o.masked);
  t.mock.method(console, 'log', () => {});
  load('util', 'fileMeta', 'icons');
  const delays = o.realSleep ? null : fastUtil();
  load('consumeView');
  return Object.assign({ env, view: window.goneConsumeView, delays }, dom);
}

function file(name, text, type) {
  const bytes = new TextEncoder().encode(text);
  return { name, type: type || 'text/plain', size: bytes.length, bytes };
}

module.exports = { setup, file, reset, load };
