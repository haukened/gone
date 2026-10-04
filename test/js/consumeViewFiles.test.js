'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { setup, file } = require('./consumeViewHarness');

test('showDecoded renders files with individual and bulk downloads', async (t) => {
  const { env, $, view, delays } = setup(t);
  const created = [];
  const revoked = [];
  t.mock.method(URL, 'createObjectURL', (b) => { created.push(b); return 'blob:' + created.length; });
  t.mock.method(URL, 'revokeObjectURL', (u) => revoked.push(u));
  const clicks = [];
  const origCreate = env.document.createElement.bind(env.document);
  env.document.createElement = (tag) => {
    const n = origCreate(tag);
    if (tag === 'a') n.addEventListener('click', function () { clicks.push([this.href, this.download, this.rel]); });
    return n;
  };

  const files = [file('a.txt', 'AAA'), file('b.bin', 'BB', 'application/octet-stream')];
  view.showDecoded({ message: '', files });
  assert.equal($('revealed-heading').textContent, 'Here\u2019s 2 files.');
  assert.equal($('message-panel').hidden, true);
  assert.equal($('file-section').hidden, false);
  assert.equal($('download-all').hidden, false);

  const items = $('file-output-list').querySelectorAll('li');
  assert.equal(items.length, 2);
  assert.equal(items[0].querySelector('.name').textContent, 'a.txt');
  assert.equal(items[0].querySelector('.size').textContent, '3 B');
  const btn = items[0].querySelector('button');
  assert.equal(btn.getAttribute('aria-label'), 'Download a.txt');
  assert.ok(btn.classList.contains('btn-secondary'));
  assert.equal(btn.querySelector('span').textContent, 'Download');

  const ev = { preventDefault() { this.prevented = true; } };
  view.guardUnload(ev);
  assert.equal(ev.prevented, true);
  assert.equal(ev.returnValue, '');

  btn.click();
  btn.click();
  assert.deepEqual(clicks, [['blob:1', 'a.txt', 'noopener'], ['blob:1', 'a.txt', 'noopener']]);
  assert.equal(created.length, 1);
  assert.equal(created[0].type, 'text/plain');
  assert.ok(items[0].classList.contains('is-done'));
  assert.equal(env.document.body.querySelectorAll('a').length, 0);

  await $('download-all').click().settled;
  assert.deepEqual(clicks.slice(2).map((c) => c[1]), ['a.txt', 'b.bin']);
  assert.deepEqual(delays, [400, 400]);
  assert.ok(items[1].classList.contains('is-done'));

  const ev2 = { preventDefault() { this.prevented = true; } };
  view.guardUnload(ev2);
  assert.equal(ev2.prevented, undefined);

  const pt = new Uint8Array([7, 7]);
  view.keepPlaintext(pt);
  view.cleanup();
  assert.deepEqual(revoked, ['blob:1', 'blob:2']);
  assert.deepEqual(pt, new Uint8Array([0, 0]));
  view.cleanup();
  assert.equal(revoked.length, 2);
});

test('a single file hides download-all; optional file elements may be absent', (t) => {
  const { $, view } = setup(t);
  view.showDecoded({ message: 'm', files: [file('a', 'x')] });
  assert.equal($('download-all').hidden, true);
  assert.equal($('revealed-heading').textContent, 'Here\u2019s your secret and a file.');
  const s = setup(t, ['download-all', 'file-section']);
  s.view.showDecoded({ message: '', files: [file('a', 'x'), file('b', 'y')] });
  assert.equal(s.$('file-output-list').children.length, 2);
  const u = setup(t, ['revealed-heading', 'view-gone']);
  u.view.showDecoded({ message: 'm', files: [] });
  assert.equal(u.$('view-revealed').hidden, false);
});

test('showGone switches to the gone view', (t) => {
  const { env, $, view } = setup(t);
  view.showGone();
  assert.equal($('view-open').hidden, true);
  assert.equal($('view-gone').hidden, false);
  assert.equal(env.document.title, 'Gone \u00b7 This secret is gone');
  assert.equal(env.document.activeElement, $('gone-heading'));
});

test('showAckResult reveals the warning only on failure', (t) => {
  const { $, view } = setup(t);
  view.showAckResult(true);
  assert.equal($('ack-warning').hidden, true);
  view.showAckResult(false);
  assert.equal($('ack-warning').hidden, false);
});
