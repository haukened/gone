/* global document */
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h } = require('./harness');

function boot(withList) {
  reset('https://gone.test/request');
  if (withList !== false) {
    document.body.append(h('ul', { id: 'request-list', hidden: true }), h('p', { id: 'request-empty' }), h('span', { id: 'request-list-status' }));
  }
  load('util', 'requestList');
  return window.goneRequestList;
}

test('render shows one linked row per entry', () => {
  const l = boot();
  const now = Date.UTC(2026, 0, 2);
  l.render([
    { id: 'a'.repeat(32), label: 'DB password', createdAt: now - 5 * 60000, state: 'waiting' },
    { id: 'b'.repeat(32), label: '', createdAt: now - 3 * 3600000, state: 'ready' }
  ], now);
  const list = document.getElementById('request-list');
  assert.equal(list.hidden, false);
  assert.equal(document.getElementById('request-empty').hidden, true);
  const [first, second] = list.children;
  const link = first.children[0];
  assert.equal(link.href, '/request/' + 'a'.repeat(32));
  assert.equal(link.children[0].textContent, 'DB password');
  assert.equal(link.children[1].textContent, 'Asked 5 minutes ago');
  assert.equal(link.children[2].dataset.state, 'waiting');
  assert.equal(second.children[0].children[0].textContent, 'Untitled request');
  assert.equal(second.children[0].children[2].textContent, 'Reply ready');
  l.render([], now);
  assert.equal(list.hidden, true);
  assert.equal(document.getElementById('request-empty').hidden, false);
  l.render([{ id: 'c'.repeat(32), createdAt: Date.now() }]);
  assert.equal(list.children.length, 1);
});

test('since uses the largest whole unit', () => {
  const l = boot();
  const since = (a, b) => { const r = l.since(a, b); return window.goneI18n.t(r.key, r.args); };
  assert.equal(since(1000, 30000), 'Asked just now');
  assert.equal(since(0, 2 * 3600000), 'Asked 2 hours ago');
  assert.equal(since(0, 3 * 86400000), 'Asked 3 days ago');
  assert.equal(since(0, 86400000), 'Asked yesterday');
  assert.equal(since(5000, 0), 'Asked just now');
});

test('announce and missing elements', () => {
  const l = boot();
  l.announce('A reply arrived.');
  assert.equal(document.getElementById('request-list-status').textContent, 'A reply arrived.');
  const bare = boot(false);
  bare.render([{ id: 'x', createdAt: 0 }]);
  bare.announce('x');
  load('requestList');
  assert.equal(window.goneRequestList, bare);
});
