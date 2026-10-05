'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { setup } = require('./consumeViewHarness');

function passphraseFixture(t) {
  const { $, view, openLabel } = setup(t);
  const btn = $('open-secret');
  const pass = $('open-passphrase');
  const toggle = $('open-pass-toggle');
  const state = { presses: 0 };
  view.onOpen(() => state.presses++);
  return { $, view, openLabel, btn, pass, toggle, state };
}

function assertShown(f) {
  const { $, view, btn } = f;
  assert.equal(view.passphraseMissing(), false);
  assert.equal(view.showPassphrase(), true);
  assert.equal($('open-pass-field').hidden, false);
  assert.equal($('open-pass-warn').hidden, false);
  assert.equal($('open-hint').hidden, true);
  assert.equal(btn.getAttribute('aria-describedby'), 'open-pass-warn');
  assert.equal(btn.getAttribute('aria-disabled'), 'true');
  assert.equal(view.passphraseMissing(), true);
}

function enterPassphrase(f) {
  const { view, btn, pass } = f;
  pass.value = 'Correct horse';
  pass.setAttribute('aria-invalid', 'true');
  pass.dispatch('input');
  assert.equal(pass.getAttribute('aria-invalid'), 'false');
  assert.equal(btn.getAttribute('aria-disabled'), 'false');
  assert.equal(view.passphrase(), 'Correct horse');
}

function toggleAndSubmit(f) {
  const { pass, toggle, state } = f;
  toggle.click();
  assert.equal(pass.type, 'text');
  assert.equal(toggle.textContent, 'Hide');
  toggle.click();
  assert.equal(pass.type, 'password');
  assert.equal(toggle.textContent, 'Show');
  assert.equal(pass.dispatch('keydown', { key: 'a' }).defaultPrevented, false);
  assert.equal(state.presses, 0);
  assert.equal(pass.dispatch('keydown', { key: 'Enter' }).defaultPrevented, true);
  assert.equal(state.presses, 1);
}

function assertOpeningFailure(f) {
  const { $, view, openLabel, pass } = f;
  view.setOpening(true);
  assert.equal(pass.readOnly, true);
  view.setOpening(false);
  assert.equal(pass.readOnly, false);
  view.passphraseFailed();
  assert.equal(openLabel.textContent, 'Try again');
  assert.equal($('open-pass-warn').hidden, true);
  assert.ok($('step-waiting').classList.contains('is-done'));
  assert.ok(!$('step-waiting').classList.contains('is-now'));
  assert.equal($('step-waiting').getAttribute('aria-current'), null);
  assert.ok($('step-opened').classList.contains('is-now'));
  assert.equal($('step-opened').getAttribute('aria-current'), 'step');
  assert.equal($('step-waiting-note').textContent, 'Done');
  assert.equal($('step-opened-note').textContent, 'Downloaded; needs the passphrase');
  assert.equal($('step-gone-note').textContent, 'When you leave this page');
  assert.equal(f.btn.getAttribute('aria-describedby'), 'consume-error-text');
  assert.equal(pass.getAttribute('aria-invalid'), 'true');
  assert.equal(globalThis.document.activeElement, pass);
  assert.equal(pass.selected, true);
  view.setOpening(true);
  assert.equal(openLabel.textContent, 'Opening\u2026');
  view.setOpening(false);
  assert.equal(openLabel.textContent, 'Try again');
}

function assertDisableAndCleanup(f) {
  const { view, btn, pass, toggle } = f;
  globalThis.document.activeElement = null;
  view.focusPassphrase();
  assert.equal(globalThis.document.activeElement, pass);

  view.disableOpen();
  assert.equal(pass.readOnly, true);
  pass.dispatch('input');
  assert.equal(btn.getAttribute('aria-disabled'), 'true');
  view.setOpening(false);
  assert.equal(pass.readOnly, true);

  toggle.click();
  view.cleanup();
  assert.equal(pass.value, '');
  assert.equal(pass.type, 'password');
  assert.equal(toggle.textContent, 'Show');
}

test('the passphrase field gates Open, toggles visibility and submits on Enter', (t) => {
  const f = passphraseFixture(t);
  assertShown(f);
  enterPassphrase(f);
  toggleAndSubmit(f);
  assertOpeningFailure(f);
  assertDisableAndCleanup(f);
});

test('showDecoded clears the passphrase', (t) => {
  const { $, view } = setup(t);
  view.showPassphrase();
  $('open-passphrase').value = 'secret words';
  view.showDecoded({ message: 'm', files: [] });
  assert.equal($('open-passphrase').value, '');
});

test('passphrase helpers tolerate missing optional elements', (t) => {
  const a = setup(t, ['open-pass-field', 'open-pass-toggle', 'open-pass-warn', 'open-secret']);
  assert.equal(a.view.showPassphrase(), true);
  assert.equal(a.$('open-secret'), null);
  a.$('open-passphrase').value = 'x';
  a.$('open-passphrase').dispatch('input');
  assert.equal(a.$('open-passphrase').dispatch('keydown', { key: 'Enter' }).defaultPrevented, true);
  a.view.clearPassphrase();

  const b = setup(t, ['open-pass-warn']);
  b.view.showPassphrase();
  assert.equal(b.$('open-secret').hasAttribute('aria-describedby'), false);

  const c = setup(t, ['open-passphrase']);
  assert.equal(c.view.showPassphrase(), false);
  assert.equal(c.$('open-pass-field').hidden, true);
  assert.equal(c.view.passphrase(), '');
  assert.equal(c.view.passphraseMissing(), false);
  assert.doesNotThrow(() => { c.view.focusPassphrase(); c.view.passphraseFailed(); c.view.clearPassphrase(); });
  assert.equal(c.openLabel.textContent, 'Try again');
});
