'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, fastUtil, h } = require('./harness');

const IDS = ['secret-heading', 'secret-output', 'copy-secret', 'download-all', 'download-progress', 'file-section',
  'file-output-list', 'ack-banner', 'ack-card', 'ack-title', 'ack-text'];
const TAGS = { 'secret-output': 'textarea', 'copy-secret': 'button', 'download-all': 'button', 'download-progress': 'progress', 'file-output-list': 'ul' };

// page builds the consume page DOM, omitting any ids in skip.
function page(env, skip) {
  const omit = new Set(skip || []);
  const root = h('section', { id: 'secret-consume' });
  IDS.filter((id) => !omit.has(id)).forEach((id) => root.appendChild(h(TAGS[id] || 'div', { id, hidden: true })));
  env.document.body.appendChild(root);
  const dom = {};
  IDS.forEach((id) => { dom[id] = env.document.getElementById(id); });
  return dom;
}

// setup loads the view against a fresh page.
function setup(t, skip, opts) {
  const env = reset('https://gone.test/secret/x');
  const dom = page(env, skip);
  t.mock.method(console, 'log', () => {});
  load('util', 'fileMeta');
  const delays = (opts && opts.realSleep) ? null : fastUtil();
  load('consumeView');
  return { env, dom, view: window.goneConsumeView, delays };
}

const file = (name, text, type) => {
  const bytes = new TextEncoder().encode(text);
  return { name, type: type || 'text/plain', size: bytes.length, bytes };
};

test('requires util and fileMeta; loads once; reports presence', (t) => {
  reset();
  load('consumeView');
  assert.equal(window.goneConsumeView, undefined);
  const { view } = setup(t);
  assert.equal(view.present, true);
  load('consumeView');
  assert.equal(window.goneConsumeView, view);
  reset();
  load('util', 'fileMeta', 'consumeView');
  assert.equal(window.goneConsumeView.present, false);
});

test('setProgress shows percent or bytes', (t) => {
  const { dom, view } = setup(t);
  const bar = dom['download-progress'];
  view.setProgress(50, 200);
  assert.equal(bar.hidden, false);
  assert.equal(bar.max, 200);
  assert.equal(bar.value, 50);
  assert.equal(dom['secret-heading'].textContent, 'Retrieving\u2026 25%');
  view.setProgress(300, 200);
  assert.equal(bar.value, 200);
  view.setProgress(2048, 0);
  assert.equal(bar.value, undefined);
  assert.equal(dom['secret-heading'].textContent, 'Retrieving\u2026 2.0 KB');
  view.hideProgress();
  assert.equal(bar.hidden, true);
});

test('progress and status tolerate missing elements', (t) => {
  const { view } = setup(t, IDS);
  assert.doesNotThrow(() => {
    view.setStatus('x');
    view.setProgress(1, 2);
    view.hideProgress();
    view.showMessage('hi');
    view.showDecoded({ message: 'm', files: [file('a', 'b')] });
    view.showAckResult(false);
  });
});

test('headingFor', (t) => {
  const { view } = setup(t);
  const f = file('a', 'x');
  const cases = [
    [{ message: 'm', files: [] }, 'Decrypted Secret:'],
    [{ message: '', files: [] }, 'Decrypted Secret:'],
    [{ message: 'm', files: [f] }, 'Decrypted Secret + 1 file:'],
    [{ message: 'm', files: [f, f] }, 'Decrypted Secret + 2 files:'],
    [{ message: '', files: [f] }, 'Decrypted 1 file:'],
    [{ message: '', files: [f, f, f] }, 'Decrypted 3 files:']
  ];
  for (const [d, want] of cases) assert.equal(view.headingFor(d), want);
});

test('showMessage fills the output, grows it and wires copy', async (t) => {
  const { env, dom, view } = setup(t);
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const out = dom['secret-output'];
  out.scrollHeight = 100;
  view.showMessage('');
  assert.equal(out.hidden, true);
  view.showMessage('secret text');
  assert.equal(out.value, 'secret text');
  assert.equal(out.hidden, false);
  assert.equal(out.style.height, '100px');
  assert.equal(out.style.overflowY, 'hidden');
  const copy = dom['copy-secret'];
  assert.equal(copy.hidden, false);
  await copy.click().settled;
  assert.equal(env.clipboard.text, 'secret text');
  assert.match(copy.innerHTML, /^Copied!/);
  t.mock.timers.tick(2200);
  assert.match(copy.innerHTML, /^Copy Secret <svg/);
  env.clipboard.fail = true;
  await copy.click().settled;
  assert.match(copy.innerHTML, /^Copy Secret/);
  assert.equal(env.alerts.length, 1);
});

test('showMessage caps tall content and works without a copy button', (t) => {
  const { dom, view } = setup(t, ['copy-secret']);
  dom['secret-output'].scrollHeight = 5000;
  view.showMessage('x');
  assert.equal(dom['secret-output'].style.height, '640px');
  assert.equal(dom['secret-output'].style.overflowY, 'auto');
});

test('showDecoded renders files with individual and bulk downloads', async (t) => {
  const { env, dom, view, delays } = setup(t);
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

  dom['download-progress'].hidden = false;
  const files = [file('a.txt', 'AAA'), file('b.bin', 'BB', 'application/octet-stream')];
  view.showDecoded({ message: 'hello', files });
  assert.equal(dom['download-progress'].hidden, true);
  assert.equal(dom['secret-heading'].textContent, 'Decrypted Secret + 2 files:');
  assert.equal(dom['secret-output'].value, 'hello');
  assert.equal(dom['file-section'].hidden, false);
  assert.equal(dom['download-all'].hidden, false);

  const items = dom['file-output-list'].querySelectorAll('li');
  assert.equal(items.length, 2);
  assert.equal(items[0].querySelector('.file-item-name').textContent, 'a.txt');
  assert.equal(items[0].querySelector('.file-item-meta').textContent, '3 B');
  const btn = items[0].querySelector('button');
  assert.equal(btn.getAttribute('aria-label'), 'Download a.txt');

  const ev = { preventDefault() { this.prevented = true; } };
  view.guardUnload(ev);
  assert.equal(ev.prevented, true);
  assert.equal(ev.returnValue, '');

  btn.click();
  btn.click();
  assert.deepEqual(clicks, [['blob:1', 'a.txt', 'noopener'], ['blob:1', 'a.txt', 'noopener']]);
  assert.equal(created.length, 1);
  assert.equal(created[0].type, 'text/plain');
  assert.ok(items[0].classList.contains('downloaded'));
  assert.equal(env.document.body.querySelectorAll('a').length, 0);

  await dom['download-all'].click().settled;
  assert.deepEqual(clicks.slice(2).map((c) => c[1]), ['a.txt', 'b.bin']);
  assert.deepEqual(delays, [400, 400]);
  assert.ok(items[1].classList.contains('downloaded'));

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

test('single file hides download-all; missing optional elements are tolerated', (t) => {
  const { dom, view } = setup(t);
  view.showDecoded({ message: '', files: [file('a', 'x')] });
  assert.equal(dom['download-all'].hidden, true);
  assert.equal(dom['secret-output'].hidden, true);

  const s = setup(t, ['download-all', 'file-section']);
  assert.doesNotThrow(() => s.view.showDecoded({ message: '', files: [file('a', 'x'), file('b', 'y')] }));
  assert.equal(s.dom['file-output-list'].children.length, 2);
  assert.doesNotThrow(() => s.view.cleanup());
});

test('showAckResult shows success or a danger warning', (t) => {
  const { dom, view } = setup(t);
  view.showAckResult(true);
  assert.equal(dom['ack-banner'].hidden, false);
  assert.equal(dom['ack-card'].classList.contains('danger'), false);

  const s = setup(t);
  s.view.showAckResult(false);
  assert.equal(s.dom['ack-banner'].hidden, false);
  assert.ok(s.dom['ack-card'].classList.contains('danger'));
  assert.equal(s.dom['ack-title'].textContent, 'Couldn\u2019t confirm deletion');
  assert.match(s.dom['ack-text'].textContent, /did not confirm/);

  const m = setup(t, ['ack-card', 'ack-title', 'ack-text']);
  m.view.showAckResult(false);
  assert.equal(m.dom['ack-banner'].hidden, false);
});
