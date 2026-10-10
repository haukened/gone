'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, loadRaw, h } = require('./harness');

reset();
loadRaw('submitMeter');
assert.equal(window.goneSizeMeter, undefined);
load('submitMeter');
const meter = window.goneSizeMeter;
const text = (p) => (p ? window.goneI18n.t(p.key, p.args) : p);

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
    [[1, 10 ** 12, 10, 0], '']
  ];
  for (const [args, want] of cases) assert.equal(text(meter.selectionProblem(...args)), want, args.join());
});

test('render updates box, meter, label and warning', () => {
  const els = { box: h('div'), meter: h('progress'), label: h('span'), warning: h('div', { hidden: true }), warningText: h('p') };
  meter.render(els, 50, 100, '');
  assert.equal(els.meter.max, 100);
  assert.equal(els.meter.value, 50);
  assert.equal(els.box.classList.contains('over'), false);
  assert.equal(els.label.textContent, '50 B of 100 B');
  assert.equal(els.warning.hidden, true);
  assert.equal(els.warningText.textContent, '');

  const tooBig = meter.selectionProblem(1, 150, 10, 100);
  meter.render(els, 150, 100, tooBig);
  assert.equal(els.meter.value, 100);
  assert.ok(els.box.classList.contains('over'));
  assert.equal(els.label.textContent, '150 B of 100 B (over limit)');
  assert.equal(els.warningText.textContent, 'Over the limit by 50 B. Remove a file or shorten the message.');
  assert.equal(els.warning.hidden, false);

  // Unchanged text is not rewritten, so the live region does not re-announce.
  let writes = 0;
  const node = els.warningText;
  Object.defineProperty(node, 'textContent', { get() { return 'x'; }, set() { writes++; }, configurable: true });
  meter.render(els, 150, 100, meter.selectionProblem(1, 150, 10, 100));
  assert.equal(writes, 0);
  delete node.textContent;

  meter.render(els, 5, 0, '');
  assert.equal(els.meter.max, 1);
  assert.equal(els.meter.value, 1);
  assert.equal(els.label.textContent, '5 B of 0 B');
  assert.equal(els.box.classList.contains('over'), false);
});

test('render tolerates missing elements', () => {
  assert.doesNotThrow(() => meter.render({}, 1, 2, { key: 'js.submit.empty' }));
});
