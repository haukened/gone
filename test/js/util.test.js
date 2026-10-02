'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, captureConsole, h } = require('./harness');

test('logTiming is silent unless timing debug is enabled', (t) => {
  reset('https://gone.test/');
  load('util');
  const logs = captureConsole(t);
  window.goneUtil.logTiming('x', 1, 2);
  assert.deepEqual(logs.log, []);
});

test('logTiming enabled via query string or localStorage', async (t) => {
  const cases = [
    { name: 'query', url: 'https://gone.test/?debug=timing', opts: {} },
    { name: 'storage', url: 'https://gone.test/', opts: { storage: { goneDebugTiming: '1' } } }
  ];
  for (const c of cases) {
    await t.test(c.name, (st) => {
      reset(c.url, c.opts);
      load('util');
      const logs = captureConsole(st);
      window.goneUtil.logTiming('upload', 10, 12.5);
      assert.deepEqual(logs.log, ['[gone][timing] upload: 2.50ms']);
    });
  }
});

test('logTiming stays off when storage access throws or is absent', async (t) => {
  for (const opts of [{ storageThrows: true }, { noStorage: true }]) {
    await t.test(JSON.stringify(opts), (st) => {
      reset('https://gone.test/', opts);
      if (opts.noStorage) globalThis.localStorage = undefined;
      load('util');
      const logs = captureConsole(st);
      window.goneUtil.logTiming('x', 0, 1);
      assert.deepEqual(logs.log, []);
    });
  }
});

test('module loads once', () => {
  reset();
  load('util');
  const first = window.goneUtil;
  load('util');
  assert.equal(window.goneUtil, first);
  assert.ok(Object.isFrozen(first));
});

test('sleep resolves after the delay', async () => {
  reset();
  load('util');
  const start = Date.now();
  await window.goneUtil.sleep(15);
  assert.ok(Date.now() - start >= 10);
});

test('allPresent and setText', () => {
  reset();
  load('util');
  const u = window.goneUtil;
  assert.equal(u.allPresent([1, 'a', {}]), true);
  assert.equal(u.allPresent([1, null]), false);
  const node = h('p');
  u.setText(node, 'hi');
  assert.equal(node.textContent, 'hi');
  assert.doesNotThrow(() => u.setText(null, 'x'));
});

test('el assigns properties and children', () => {
  reset();
  load('util');
  const u = window.goneUtil;
  const child = u.el('span', { textContent: 'c' });
  const node = u.el('div', { id: 'p', className: 'a b' }, [child]);
  assert.equal(node.tagName, 'DIV');
  assert.equal(node.id, 'p');
  assert.ok(node.classList.contains('b'));
  assert.equal(node.children[0], child);
  assert.equal(u.el('i', {}).children.length, 0);
});

test('setContent replaces children with nodes and text', () => {
  reset();
  load('util');
  const old = h('b');
  const node = h('p', {}, [old]);
  const icon = h('svg');
  window.goneUtil.setContent(node, ['<b>x</b> ', icon]);
  assert.equal(old.parentNode, null);
  assert.equal(node.textContent, '<b>x</b> ');
  assert.equal(node.children[0].nodeType, 3);
  assert.equal(node.children[1], icon);
});

test('flashCopied swaps content then restores it', (t) => {
  reset();
  load('util');
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const icon = h('svg');
  const btn = h('button', {}, [icon]);
  window.goneUtil.flashCopied(btn, ['done ', h('i')]);
  assert.equal(btn.textContent, 'done ');
  assert.equal(btn.children[1].tagName, 'I');
  assert.equal(icon.parentNode, null);
  assert.ok(btn.classList.contains('copied'));
  assert.equal(btn.disabled, true);
  t.mock.timers.tick(2200);
  assert.deepEqual(btn.children, [icon]);
  assert.equal(icon.parentNode, btn);
  assert.equal(btn.classList.contains('copied'), false);
  assert.equal(btn.disabled, false);
});

test('copyText reports success and failure', async () => {
  const env = reset();
  load('util');
  const u = window.goneUtil;
  assert.equal(await u.copyText('abc'), true);
  assert.equal(env.clipboard.text, 'abc');
  env.clipboard.fail = true;
  let called = 0;
  assert.equal(await u.copyText('x', () => { called++; }), false);
  assert.equal(await u.copyText('x'), false);
  assert.equal(called, 1);
  assert.equal(env.alerts.length, 2);
  assert.match(env.alerts[0], /Copy failed/);
});
