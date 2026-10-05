'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { setup } = require('./consumeViewHarness');

const masked = (t, env) => setup(t, [], { masked: true, env });

test('a revealed message starts covered: size shown, text kept out of the page', (t) => {
  const { $, view } = masked(t);
  view.showDecoded({ message: 'line one\nline two\nline three\n', files: [] });
  assert.equal($('secret-output').hidden, true);
  assert.equal($('secret-output').textContent, '');
  assert.equal($('secret-cover').hidden, false);
  assert.equal($('secret-size').textContent, '3 lines');
  assert.equal($('secret-cover-dots').textContent.split('\n').length, 3);
  assert.equal($('hide-secret').hidden, true);
  assert.equal($('always-show').checked, false);
});

test('Show and Hide swap the cover for the message and move focus', (t) => {
  const { env, $, view } = masked(t);
  view.showDecoded({ message: 'hunter2', files: [] });
  $('show-secret').click();
  assert.equal($('secret-output').textContent, 'hunter2');
  assert.equal($('secret-output').hidden, false);
  assert.equal($('secret-cover').hidden, true);
  assert.equal($('hide-secret').hidden, false);
  assert.equal(env.document.activeElement, $('secret-output'));
  $('hide-secret').click();
  assert.equal($('secret-output').textContent, '');
  assert.equal($('secret-cover').hidden, false);
  assert.equal($('hide-secret').hidden, true);
  assert.equal(env.document.activeElement, $('show-secret'));
});

test('Copy works while covered; the hand-copy fallback shows the message first', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const { env, $, view } = masked(t);
  view.showDecoded({ message: 'secret', files: [] });
  await $('copy-secret').click().settled;
  assert.equal(env.clipboard.text, 'secret');
  assert.equal($('secret-output').hidden, true);
  t.mock.timers.tick(2200);
  env.clipboard.fail = true;
  await $('copy-secret').click().settled;
  assert.equal($('secret-output').hidden, false);
  assert.equal($('hide-secret').hidden, false);
  assert.equal(env.selection.ranges[0].node, $('secret-output'));
});

test('"Always show" is remembered on this device and shows the message at once', (t) => {
  const { env, $, view } = masked(t);
  view.showDecoded({ message: 'x', files: [] });
  $('always-show').checked = true;
  $('always-show').dispatch('change');
  assert.equal(env.storage.goneAlwaysShow, '1');
  assert.equal($('secret-output').textContent, 'x');
  $('always-show').checked = false;
  $('always-show').dispatch('change');
  assert.equal('goneAlwaysShow' in env.storage, false);
  assert.equal($('secret-output').hidden, false);

  const again = masked(t, { storage: { goneAlwaysShow: '1' } });
  again.view.showDecoded({ message: 'y', files: [] });
  assert.equal(again.$('secret-output').textContent, 'y');
  assert.equal(again.$('always-show').checked, true);
});

test('blocked storage leaves the message covered and the box still works', (t) => {
  const { $, view } = masked(t, { storageThrows: true });
  view.showDecoded({ message: 'z', files: [] });
  assert.equal($('secret-output').hidden, true);
  $('always-show').checked = true;
  $('always-show').dispatch('change');
  assert.equal($('secret-output').textContent, 'z');
});

test('sizeOf counts lines, or characters for a single line', (t) => {
  const { view } = masked(t);
  const sizeOf = window.goneConsumeViewMask.sizeOf;
  assert.ok(view);
  assert.equal(sizeOf('a'), '1 character');
  assert.equal(sizeOf('päss🔑'), '5 characters');
  assert.equal(sizeOf('a\nb'), '2 lines');
  assert.equal(sizeOf('a\nb\n\n'), '2 lines');
  assert.equal(sizeOf('1\n2\n3\n4\n5\n6\n7'), '7 lines');
});

test('more lines than placeholder rows caps the dots', (t) => {
  const { $, view } = masked(t);
  view.showDecoded({ message: '1\n2\n3\n4\n5\n6\n7\n8', files: [] });
  assert.equal($('secret-cover-dots').textContent.split('\n').length, 5);
});
