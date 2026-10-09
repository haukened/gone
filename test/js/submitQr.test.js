'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h } = require('./harness');

const SVG_NS = 'http://www.w3.org/2000/svg';
const LINK = 'https://gone.test/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa#v1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';

// parts builds the QR button, figure and code box around a link input.
function parts() {
  reset();
  load('qr', 'submitQr');
  const el = {
    button: h('button', { hidden: true, attrs: { 'aria-expanded': 'false' } }),
    figure: h('figure', { hidden: true }),
    code: h('div'),
    input: h('input', { value: LINK })
  };
  el.figure.appendChild(el.code);
  return el;
}

// modulesFromPath paints the path's runs back into a size*size grid.
function modulesFromPath(d, size) {
  const grid = new Uint8Array(size * size);
  for (const m of d.matchAll(/M(\d+) (\d+)h(\d+)v1h-(\d+)z/g)) {
    const [x, y, len, back] = m.slice(1).map(Number);
    assert.equal(len, back);
    for (let i = 0; i < len; i++) grid[(y - 4) * size + x - 4 + i] = 1;
  }
  return grid;
}

test('requires the encoder; loads once', () => {
  reset();
  load('submitQr');
  assert.equal(window.goneResultQr, undefined);
  load('qr', 'submitQr');
  const qr = window.goneResultQr;
  assert.ok(Object.isFrozen(qr));
  load('submitQr');
  assert.equal(window.goneResultQr, qr);
});

test('svgFor draws every dark module inside a 4-module quiet zone', () => {
  parts();
  const code = window.goneQr.encode(LINK);
  const svg = window.goneResultQr.svgFor(LINK);
  const n = code.size + 8;
  assert.equal(svg.namespaceURI, SVG_NS);
  assert.equal(svg.getAttribute('viewBox'), `0 0 ${n} ${n}`);
  assert.equal(svg.getAttribute('shape-rendering'), 'crispEdges');
  assert.equal(svg.getAttribute('aria-hidden'), 'true');
  const [bg, path] = svg.children;
  assert.equal(bg.tagName, 'RECT');
  assert.equal(bg.getAttribute('class'), 'qr-light');
  assert.equal(bg.getAttribute('width'), String(n));
  assert.equal(path.getAttribute('class'), 'qr-dark');
  assert.deepEqual(modulesFromPath(path.getAttribute('d'), code.size), code.dark);
});

test('the button starts closed and toggles the code open and shut', () => {
  const el = parts();
  let scrolled = null;
  el.figure.scrollIntoView = (opts) => { scrolled = opts; };
  window.goneResultQr.attach(el);
  window.goneResultQr.attach(el);
  assert.equal(el.button.hidden, false);
  assert.equal(el.button.getAttribute('aria-expanded'), 'false');
  assert.equal(el.figure.hidden, true);
  el.button.click();
  assert.equal(el.button.getAttribute('aria-expanded'), 'true');
  assert.equal(el.figure.hidden, false);
  assert.equal(el.code.children.length, 1);
  assert.equal(el.code.children[0].tagName, 'SVG');
  assert.deepEqual(scrolled, { block: 'nearest' });
  el.button.click();
  assert.equal(el.button.getAttribute('aria-expanded'), 'false');
  assert.equal(el.figure.hidden, true);
  assert.equal(el.code.children.length, 0);
});

test('attach closes an open code for the next link', () => {
  const el = parts();
  window.goneResultQr.attach(el);
  el.button.click();
  el.input.value = LINK + 'B';
  window.goneResultQr.attach(el);
  assert.equal(el.figure.hidden, true);
  assert.equal(el.code.children.length, 0);
  el.button.click();
  const d = el.code.children[0].children[1].getAttribute('d');
  assert.equal(d, window.goneResultQr.svgFor(LINK + 'B').children[1].getAttribute('d'));
});

test('a link the encoder cannot draw hides the button', () => {
  const el = parts();
  window.goneResultQr.attach(el);
  window.goneQr = { encode() { throw new RangeError('too long'); } };
  el.button.click();
  assert.equal(el.button.hidden, true);
  assert.equal(el.figure.hidden, true);
  assert.equal(el.button.getAttribute('aria-expanded'), 'false');
});
