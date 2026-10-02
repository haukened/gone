'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h } = require('./harness');

test('does nothing without the secret textarea', () => {
  reset();
  assert.doesNotThrow(() => load('autoResize'));
});

test('grows with content up to 40rem, then scrolls', () => {
  const env = reset();
  const ta = h('textarea', { id: 'secret', scrollHeight: 120 });
  env.document.body.appendChild(ta);
  load('autoResize');
  assert.equal(ta.style.height, '120px');
  assert.equal(ta.style.overflowY, 'hidden');
  ta.scrollHeight = 2000;
  ta.dispatch('input');
  assert.equal(ta.style.height, '640px');
  assert.equal(ta.style.overflowY, 'auto');
});
