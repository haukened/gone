'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, fastUtil, h } = require('./harness');

// page builds the three consume views, omitting any ids in skip.
function page(env, skip) {
  const omit = new Set(skip || []);
  // n builds a node; an omitted id still keeps its children in an anonymous wrapper.
  const n = (id, tag, props, kids) => {
    const children = (kids || []).filter(Boolean);
    if (!omit.has(id)) return h(tag, Object.assign({ id }, props), children);
    return children.length ? h('div', {}, children) : null;
  };
  const openLabel = h('span', { textContent: 'Open secret' });
  const copyLabel = h('span', { textContent: 'Copy message' });
  const views = [
    n('view-open', 'div', {}, [
      n('open-heading', 'h1'),
      n('open-secret', 'button', {}, [h('svg'), openLabel]),
      n('download-progress', 'progress', { hidden: true }),
      n('consume-status', 'span'),
      n('consume-error', 'div', { hidden: true }, [n('consume-error-text', 'p')])
    ]),
    n('view-revealed', 'div', { hidden: true }, [
      n('revealed-heading', 'h1'),
      n('ack-warning', 'div', { hidden: true }),
      n('message-panel', 'div', { hidden: true }, [n('copy-secret', 'button', {}, [h('svg'), copyLabel]), n('secret-output', 'div')]),
      n('copy-status', 'span'),
      n('file-section', 'div', { hidden: true }, [n('file-output-list', 'ul'), n('download-all', 'button', { hidden: true })])
    ]),
    n('view-gone', 'div', { hidden: true }, [n('gone-heading', 'h1')])
  ];
  env.document.body.append(...views.filter(Boolean));
  const $ = (id) => env.document.getElementById(id);
  return { $, openLabel, copyLabel };
}

// setup loads the view against a fresh page.
function setup(t, skip, opts) {
  const env = reset('https://gone.test/secret/x');
  const dom = page(env, skip);
  t.mock.method(console, 'log', () => {});
  load('util', 'fileMeta', 'icons');
  const delays = (opts && opts.realSleep) ? null : fastUtil();
  load('consumeView');
  return Object.assign({ env, view: window.goneConsumeView, delays }, dom);
}

const file = (name, text, type) => {
  const bytes = new TextEncoder().encode(text);
  return { name, type: type || 'text/plain', size: bytes.length, bytes };
};

test('requires util, fileMeta and icons; loads once; reports presence', (t) => {
  for (const deps of [[], ['util', 'fileMeta'], ['util', 'icons'], ['fileMeta', 'icons']]) {
    reset();
    load(...deps, 'consumeView');
    assert.equal(window.goneConsumeView, undefined, deps.join());
  }
  const { view } = setup(t);
  assert.equal(view.present, true);
  assert.ok(Object.isFrozen(view));
  load('consumeView');
  assert.equal(window.goneConsumeView, view);
  reset();
  load('util', 'fileMeta', 'icons', 'consumeView');
  assert.equal(window.goneConsumeView.present, false);
});

test('setProgress shows percent, or bytes when the total is unknown', (t) => {
  const { $, view } = setup(t);
  const bar = $('download-progress');
  view.setProgress(50, 200);
  assert.equal(bar.hidden, false);
  assert.equal(bar.max, 200);
  assert.equal(bar.value, 50);
  assert.equal($('consume-status').textContent, 'Retrieving\u2026 25%');
  view.setProgress(500, 200);
  assert.equal(bar.value, 200);
  bar.setAttribute('value', '3');
  view.setProgress(2048, 0);
  assert.equal(bar.hasAttribute('value'), false);
  assert.equal($('consume-status').textContent, 'Retrieving\u2026 2.0 KB');
  view.hideProgress();
  assert.equal(bar.hidden, true);
});

test('errors, opening state and the Open button', (t) => {
  const { $, view, openLabel } = setup(t);
  const btn = $('open-secret');
  let presses = 0;
  view.onOpen(() => presses++);
  btn.click();
  assert.equal(presses, 1);

  view.setOpening(true);
  assert.equal(btn.getAttribute('aria-disabled'), 'true');
  assert.equal(btn.getAttribute('aria-busy'), 'true');
  assert.equal(openLabel.textContent, 'Opening\u2026');
  view.setOpening(false);
  assert.equal(btn.getAttribute('aria-disabled'), 'false');
  assert.equal(btn.hasAttribute('aria-busy'), false);
  assert.equal(openLabel.textContent, 'Open secret');

  view.setProgress(1, 2);
  view.showError('nope');
  assert.equal($('consume-error').hidden, false);
  assert.equal($('consume-error-text').textContent, 'nope');
  assert.equal($('download-progress').hidden, true);
  assert.equal($('consume-status').textContent, '');
  view.clearError();
  assert.equal($('consume-error').hidden, true);
  view.disableOpen();
  assert.equal(btn.getAttribute('aria-disabled'), 'true');
});

test('every helper tolerates a page with no elements', (t) => {
  const env = reset('https://gone.test/secret/x');
  t.mock.method(console, 'log', () => {});
  load('util', 'fileMeta', 'icons', 'consumeView');
  const view = window.goneConsumeView;
  assert.doesNotThrow(() => {
    view.onOpen(() => {});
    view.setOpening(true);
    view.disableOpen();
    view.setProgress(1, 2);
    view.hideProgress();
    view.showError('x');
    view.clearError();
    view.showDecoded({ message: 'm', files: [file('a', 'x')] });
    view.showGone();
    view.showAckResult(false);
  });
  assert.equal(env.document.title, 'Gone \u00b7 This secret is gone');
});

test('headingFor', () => {
  reset();
  load('util', 'fileMeta', 'icons', 'consumeView');
  const f = (n) => Array.from({ length: n }, () => ({}));
  const cases = [
    [{ message: 'm', files: [] }, 'Here\u2019s your secret.'],
    [{ message: 'm', files: f(1) }, 'Here\u2019s your secret and a file.'],
    [{ message: 'm', files: f(3) }, 'Here\u2019s your secret and 3 files.'],
    [{ message: '', files: f(1) }, 'Here\u2019s a file.'],
    [{ message: '', files: f(2) }, 'Here\u2019s 2 files.']
  ];
  for (const [d, want] of cases) assert.equal(window.goneConsumeView.headingFor(d), want);
});

test('showDecoded with a message reveals it, titles the page and focuses the heading', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const { env, $, view, copyLabel } = setup(t);
  view.setProgress(1, 2);
  view.showDecoded({ message: 'hello', files: [] });
  assert.equal($('view-open').hidden, true);
  assert.equal($('view-revealed').hidden, false);
  assert.equal($('view-gone').hidden, true);
  assert.equal($('download-progress').hidden, true);
  assert.equal($('revealed-heading').textContent, 'Here\u2019s your secret.');
  assert.equal(env.document.title, 'Gone \u00b7 Here\u2019s your secret');
  assert.equal(env.document.activeElement, $('revealed-heading'));
  assert.equal($('secret-output').textContent, 'hello');
  assert.equal($('message-panel').hidden, false);
  assert.equal($('file-section').hidden, true);

  await $('copy-secret').click().settled;
  assert.equal(env.clipboard.text, 'hello');
  assert.equal(copyLabel.textContent, 'Copied');
  assert.equal($('copy-status').textContent, 'Message copied to clipboard.');
  t.mock.timers.tick(2200);

  env.clipboard.fail = true;
  await $('copy-secret').click().settled;
  assert.equal(env.selection.ranges.length, 1);
  assert.equal(env.selection.ranges[0].node, $('secret-output'));
  assert.match($('copy-status').textContent, /Ctrl\+C/);
});

test('showMessage ignores empty text and works without a copy button or panel', (t) => {
  const { $, view } = setup(t);
  view.showMessage('');
  assert.equal($('message-panel').hidden, true);
  const s = setup(t, ['copy-secret', 'message-panel']);
  s.view.showMessage('x');
  assert.equal(s.$('secret-output').textContent, 'x');
  const u = setup(t, ['secret-output']);
  u.view.showMessage('x');
  assert.equal(u.$('message-panel').hidden, true);
});

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
