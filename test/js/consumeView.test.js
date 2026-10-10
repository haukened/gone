'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { setup, file, reset, load } = require('./consumeViewHarness');

test('requires util and icons; loads once; reports presence', (t) => {
  for (const deps of [[], ['util'], ['icons']]) {
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
  for (const [d, want] of cases) {
    const h = window.goneConsumeView.headingFor(d);
    assert.equal(window.goneI18n.t(h.key, h.args), want);
  }
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

