'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h } = require('./harness');

// fields builds the passphrase controls; skip omits named keys.
function fields(skip) {
  const s = new Set(skip || []);
  const all = {
    disclosure: h('details', { id: 'pass-disclosure', open: false }),
    input: h('input', { id: 'passphrase', type: 'password', value: '' }),
    toggle: h('button', { id: 'pass-toggle' }, [h('span', { textContent: 'Show' })]),
    generate: h('button', { id: 'pass-generate' }),
    strength: h('span', { id: 'pass-strength' })
  };
  Object.keys(all).forEach((k) => { if (s.has(k)) delete all[k]; });
  return all;
}

// boot loads the field module with its dependencies and creates a field.
function boot(skip) {
  const env = reset();
  load('util', 'crypto', 'wordlist', 'passgen', 'submitPassphrase');
  const els = fields(skip);
  let changes = 0;
  const field = window.gonePassphraseField.create(els, () => { changes += 1; });
  return { env, els, field, changes: () => changes };
}

const typeIn = (b, v) => { b.els.input.value = v; b.els.input.dispatch('input'); };

test('module needs its dependencies and loads once', () => {
  reset();
  load('util', 'crypto', 'submitPassphrase');
  assert.equal(window.gonePassphraseField, undefined);
  const b = boot();
  const mod = window.gonePassphraseField;
  load('submitPassphrase');
  assert.equal(window.gonePassphraseField, mod);
  assert.equal(mod.create({}, () => {}), null);
  assert.ok(b.field);
});

test('typing rates strength, validates length and reports overhead', () => {
  const b = boot();
  assert.equal(b.els.strength.textContent, '');
  assert.equal(b.els.strength.dataset.level, 'empty');
  assert.equal(b.field.problem(), '');
  assert.equal(b.field.overhead(), 0);
  typeIn(b, 'abc');
  assert.equal(b.changes(), 1);
  assert.equal(b.els.input.getAttribute('aria-invalid'), 'true');
  assert.match(b.els.strength.textContent, /^Too short: use at least 8/);
  assert.match(b.field.problem(), /at least 8 characters, or leave it empty/);
  assert.equal(b.field.overhead(), 21);
  typeIn(b, 'password');
  assert.equal(b.els.input.getAttribute('aria-invalid'), 'false');
  assert.equal(b.els.strength.dataset.level, 'weak');
  assert.equal(b.els.strength.textContent, 'About 33 bits of entropy, about 1 hour to guess. Longer is better, or press Generate.');
  typeIn(b, 'Abcdefg1');
  assert.equal(b.els.strength.dataset.level, 'fair');
  assert.equal(b.els.strength.textContent, 'About 48 bits of entropy, about 3 years to guess.');
  typeIn(b, 'aaaaaaaaaaaa');
  assert.equal(b.els.strength.textContent, 'About 5 bits of entropy, guessed almost instantly. Longer is better, or press Generate.');
  typeIn(b, 'correct horse battery staple');
  assert.equal(b.els.strength.textContent, 'About 52 bits of entropy, about 58 years to guess.');
  typeIn(b, 'Correct-Horse-Battery-Staple-Ocean-Violet');
  assert.match(b.els.strength.textContent, /^About 78 bits of entropy, about \d+ billion years to guess\.$/);
  typeIn(b, 'x7#Qp!vL9@zR2$mW8^kT4&nY6*bH3%fJ');
  assert.equal(b.els.strength.textContent, 'About 210 bits of entropy, longer than the age of the universe to guess.');
  typeIn(b, 'lowercaseonly');
  assert.equal(b.field.value(), 'lowercaseonly');
});

test('show toggle and Generate reveal the passphrase', () => {
  const b = boot();
  const label = b.els.toggle.querySelector('span');
  b.els.toggle.click();
  assert.equal(b.els.input.type, 'text');
  assert.equal(label.textContent, 'Hide');
  b.els.toggle.click();
  assert.equal(b.els.input.type, 'password');
  assert.equal(label.textContent, 'Show');
  b.els.generate.click();
  assert.match(b.field.value(), /^([A-Z][a-z]*(-[a-z]+)?){5}$/);
  assert.equal(b.els.input.type, 'text');
  assert.equal(b.els.strength.textContent, 'About 52 bits of entropy, about 58 years to guess.');
  assert.equal(b.changes(), 1);
});

test('busy, clear and reveal manage the field state', () => {
  const b = boot();
  b.els.generate.click();
  b.field.setBusy(true);
  assert.equal(b.els.input.readOnly, true);
  assert.equal(b.els.toggle.disabled, true);
  assert.equal(b.els.generate.disabled, true);
  b.field.setBusy(false);
  assert.equal(b.els.generate.disabled, false);
  b.field.clear();
  assert.equal(b.field.value(), '');
  assert.equal(b.els.input.type, 'password');
  assert.equal(b.els.strength.textContent, '');
  b.field.reveal();
  assert.equal(b.els.disclosure.open, true);
  assert.equal(b.env.document.activeElement, b.els.input);
});

test('optional controls may be absent', () => {
  const b = boot(['disclosure', 'toggle', 'generate', 'strength']);
  typeIn(b, 'abc');
  assert.equal(b.els.input.getAttribute('aria-invalid'), 'true');
  b.field.setBusy(true);
  b.field.clear();
  b.field.reveal();
  assert.equal(b.env.document.activeElement, b.els.input);
});
