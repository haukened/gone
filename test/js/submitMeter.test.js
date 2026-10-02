'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h } = require('./harness');

reset();
load('submitMeter');
assert.equal(window.goneSizeMeter, undefined);
load('fileMeta', 'submitMeter');
const meter = window.goneSizeMeter;

test('loads once', () => {
  load('submitMeter');
  assert.equal(window.goneSizeMeter, meter);
});

test('selectionProblem', () => {
  const cases = [
    [[1, 10, 10, 100], ''],
    [[10, 100, 10, 100], ''],
    [[12, 10, 10, 100], 'Too many files: remove 2 to stay within 10.'],
    [[1, 2148, 10, 100], 'Over the limit by 2.0 KB. Remove a file or shorten the message.'],
    [[1, 1e12, 10, 0], '']
  ];
  for (const [args, want] of cases) assert.equal(meter.selectionProblem(...args), want, args.join());
});

test('render updates meter, label and warning', () => {
  const els = { meter: h('progress'), label: h('span'), warning: h('div', { hidden: true }) };
  meter.render(els, 50, 100, '');
  assert.equal(els.meter.max, 100);
  assert.equal(els.meter.value, 50);
  assert.equal(els.meter.classList.contains('over'), false);
  assert.equal(els.label.textContent, '50 B of 100 B');
  assert.equal(els.warning.hidden, true);

  meter.render(els, 150, 100, 'too big');
  assert.equal(els.meter.value, 100);
  assert.ok(els.meter.classList.contains('over'));
  assert.equal(els.label.textContent, '150 B of 100 B (over limit)');
  assert.equal(els.warning.textContent, 'too big');
  assert.equal(els.warning.hidden, false);

  meter.render(els, 5, 0, '');
  assert.equal(els.meter.max, 1);
  assert.equal(els.meter.value, 1);
  assert.equal(els.label.textContent, '5 B of 0 B');
});

test('render tolerates missing elements', () => {
  assert.doesNotThrow(() => meter.render({}, 1, 2, 'x'));
});
