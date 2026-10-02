'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');

const SVG_NS = 'http://www.w3.org/2000/svg';

test('loads once', () => {
  reset();
  load('icons');
  const icons = window.goneIcons;
  assert.ok(Object.isFrozen(icons));
  load('icons');
  assert.equal(window.goneIcons, icons);
});

test('make builds a decorative SVG from DOM nodes', () => {
  reset();
  load('icons');
  const svg = window.goneIcons.make('copy', '24');
  assert.equal(svg.namespaceURI, SVG_NS);
  assert.equal(svg.tagName, 'SVG');
  assert.equal(svg.getAttribute('width'), '24');
  assert.equal(svg.getAttribute('height'), '24');
  assert.equal(svg.getAttribute('viewBox'), '0 0 24 24');
  assert.equal(svg.getAttribute('stroke'), 'currentColor');
  assert.equal(svg.getAttribute('aria-hidden'), 'true');
  assert.deepEqual(svg.children.map((c) => c.tagName), ['RECT', 'PATH']);
  assert.ok(svg.children.every((c) => c.namespaceURI === SVG_NS));
  assert.equal(svg.children[0].getAttribute('rx'), '2');
});

test('make supports every icon and defaults the size', () => {
  reset();
  load('icons');
  const counts = { copy: 2, check: 1, back: 2, warn: 3 };
  Object.entries(counts).forEach(([name, n]) => {
    const svg = window.goneIcons.make(name);
    assert.equal(svg.getAttribute('width'), '1em');
    assert.equal(svg.children.length, n);
  });
  assert.notEqual(window.goneIcons.make('check'), window.goneIcons.make('check'));
});

test('make rejects unknown icons', () => {
  reset();
  load('icons');
  assert.throws(() => window.goneIcons.make('<img onerror=x>'), /unknown icon/);
});
