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

test('flashCopied relabels and announces, then restores both', (t) => {
  reset();
  load('util');
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const label = h('span', { textContent: 'Copy link', attrs: { 'data-i18n': 'result.copy' } });
  const btn = h('button', {}, [h('svg'), label]);
  const status = h('span');
  const u = window.goneUtil;
  u.flashCopied(btn, status, 'js.result.linkCopied');
  assert.equal(label.textContent, 'Copied');
  assert.equal(status.textContent, 'Link copied to clipboard.');
  assert.equal(btn.disabled, false);
  t.mock.timers.tick(1000);
  u.flashCopied(btn, status, 'js.result.manageCopied');
  assert.equal(status.textContent, 'Manage link copied to clipboard.');
  t.mock.timers.tick(2199);
  assert.equal(label.textContent, 'Copied');
  t.mock.timers.tick(1);
  assert.equal(label.textContent, 'Copy link');
  assert.equal(label.getAttribute('data-i18n'), 'result.copy');
  assert.equal(status.textContent, '');
  u.flashCopied(btn, null, 'js.result.linkCopied');
  assert.equal(label.textContent, 'Copied');
  t.mock.timers.tick(2200);
  assert.equal(label.textContent, 'Copy link');
});

test('flashCopied restores an untranslated label as text', (t) => {
  reset();
  load('util');
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const label = h('span', { textContent: 'Copy' });
  const btn = h('button', {}, [label]);
  window.goneUtil.flashCopied(btn, null, 'js.result.linkCopied');
  assert.equal(label.textContent, 'Copied');
  t.mock.timers.tick(2200);
  assert.equal(label.textContent, 'Copy');
  assert.equal(label.hasAttribute('data-i18n'), false);
});

test('copyText reports success, and on failure selects and explains', async () => {
  const env = reset();
  load('util');
  const u = window.goneUtil;
  const status = h('span');
  assert.equal(await u.copyText('abc', null, status), true);
  assert.equal(env.clipboard.text, 'abc');
  assert.equal(status.textContent, '');
  env.clipboard.fail = true;
  let called = 0;
  assert.equal(await u.copyText('x', () => { called++; }, status), false);
  assert.equal(called, 1);
  assert.match(status.textContent, /press Ctrl\+C/);
  assert.equal(await u.copyText('x'), false);
  assert.deepEqual(env.alerts, []);
});

test('isToken is 43 characters of strict base64url', () => {
  reset('https://gone.test/');
  load('util', 'crypto');
  const u = window.goneUtil;
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';
  for (const last of alphabet) {
    const token = 'aZ09_-'.repeat(7) + last;
    let strict = true;
    try {
      window.goneCrypto.b64urlDecode(token);
    } catch {
      strict = false;
    }
    assert.equal(u.isToken(token), strict, token);
  }
  for (const bad of ['A'.repeat(42), 'A'.repeat(44), 'A'.repeat(42) + '=', 'A'.repeat(41) + '+A', null, 42]) {
    assert.equal(u.isToken(bad), false, String(bad));
  }
});

test('clearFragment drops the fragment but keeps path and query', (t) => {
  const env = reset('https://gone.test/secret/abc?debug=timing#v1:KEY');
  load('util');
  window.goneUtil.clearFragment();
  assert.deepEqual(env.replaced, ['/secret/abc?debug=timing']);
  assert.equal(globalThis.location.hash, '');
  assert.equal(globalThis.location.search, '?debug=timing');

  const logs = captureConsole(t);
  globalThis.history = { replaceState() { throw new Error('blocked'); } };
  window.goneUtil.clearFragment();
  assert.equal(logs.warn.length, 1);
  assert.match(logs.warn[0], /could not clear/);
});

test('clearFragment waits for the load event while the page is loading', () => {
  const env = reset('https://gone.test/secret/abc#v1:KEY', { readyState: 'interactive' });
  load('util');
  window.goneUtil.clearFragment();
  assert.deepEqual(env.replaced, []);
  assert.equal(globalThis.location.hash, '#v1:KEY');
  env.windowListeners.load();
  assert.deepEqual(env.replaced, ['/secret/abc']);
  assert.equal(globalThis.location.hash, '');
});
