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

test('make builds a decorative SVG that uses the sprite symbol', () => {
  reset();
  load('icons');
  const svg = window.goneIcons.make('clip');
  assert.equal(svg.namespaceURI, SVG_NS);
  assert.equal(svg.tagName, 'SVG');
  assert.equal(svg.getAttribute('class'), 'ico');
  assert.equal(svg.getAttribute('aria-hidden'), 'true');
  assert.equal(svg.getAttribute('focusable'), 'false');
  assert.equal(svg.children.length, 1);
  const use = svg.children[0];
  assert.equal(use.tagName, 'USE');
  assert.equal(use.namespaceURI, SVG_NS);
  assert.equal(use.getAttribute('href'), '#i-clip');
  assert.notEqual(window.goneIcons.make('clip'), svg);
});

test('make rejects anything but lowercase letters', () => {
  reset();
  load('icons');
  for (const bad of ['', 'Clip', 'a-b', '"><img onerror=x>', 'x" href="javascript:1', null, 7]) {
    assert.throws(() => window.goneIcons.make(bad), /invalid icon/, String(bad));
  }
});
