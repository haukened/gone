'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h, captureConsole } = require('./harness');

const KEY = 'gone.theme';

// boot builds the theme toggle and two warnings, then loads theme.js.
// o.dark sets the system preference; o.mq === false removes matchMedia.
function boot(t, opts) {
  const o = opts || {};
  const env = reset(o.url || 'https://gone.test/', { storage: o.storage, storageThrows: o.storageThrows, readyState: o.readyState });
  const btn = h('button', { id: 'theme-toggle', hidden: true });
  const warnings = [h('div', { className: 'security-warning', hidden: true }), h('div', { className: 'security-warning', hidden: true })];
  if (!o.noToggle) env.document.body.append(btn);
  env.document.body.append(...warnings);
  const mq = { matches: Boolean(o.dark), listeners: [], addEventListener(type, fn) { this.listeners.push({ type, fn }); } };
  if (o.mq !== false) globalThis.matchMedia = (q) => { mq.query = q; return mq; };
  const logs = captureConsole(t);
  load('theme');
  return { env, btn, warnings, mq, logs, root: env.document.documentElement };
}

test('applies a valid stored theme before init and ignores junk', (t) => {
  for (const mode of ['dark', 'light']) {
    const b = boot(t, { storage: { [KEY]: mode }, dark: mode === 'light' });
    assert.equal(b.root.dataset.theme, mode);
    assert.equal(window.goneTheme.effective(), mode);
    assert.equal(b.btn.getAttribute('aria-pressed'), String(mode === 'dark'));
  }
  const b = boot(t, { storage: { [KEY]: 'neon' } });
  assert.equal(b.root.dataset.theme, undefined);
});

test('loads once and exposes a frozen API', (t) => {
  boot(t);
  const api = window.goneTheme;
  assert.ok(Object.isFrozen(api));
  load('theme');
  assert.equal(window.goneTheme, api);
});

test('without a stored choice the toggle reflects the system preference', (t) => {
  for (const dark of [true, false]) {
    const b = boot(t, { dark });
    assert.equal(b.btn.hidden, false);
    assert.equal(b.mq.query, '(prefers-color-scheme: dark)');
    assert.equal(b.root.dataset.theme, undefined);
    assert.equal(b.btn.getAttribute('aria-pressed'), String(dark));
  }
});

test('clicking toggles, stores and syncs aria-pressed', (t) => {
  const b = boot(t, { dark: true });
  b.btn.click();
  assert.equal(b.root.dataset.theme, 'light');
  assert.equal(b.env.storage[KEY], 'light');
  assert.equal(b.btn.getAttribute('aria-pressed'), 'false');
  b.btn.click();
  assert.equal(b.root.dataset.theme, 'dark');
  assert.equal(b.env.storage[KEY], 'dark');
  assert.equal(b.btn.getAttribute('aria-pressed'), 'true');
});

test('a system change re-syncs the toggle while no choice is applied', (t) => {
  const b = boot(t, { dark: false });
  assert.equal(b.mq.listeners[0].type, 'change');
  b.mq.matches = true;
  b.mq.listeners[0].fn();
  assert.equal(b.btn.getAttribute('aria-pressed'), 'true');
});

test('works without matchMedia or storage', (t) => {
  const b = boot(t, { mq: false, storageThrows: true });
  assert.equal(window.goneTheme.effective(), 'light');
  assert.equal(b.btn.getAttribute('aria-pressed'), 'false');
  b.btn.click();
  assert.equal(b.root.dataset.theme, 'dark');
  assert.equal(b.btn.getAttribute('aria-pressed'), 'true');
});

test('a missing toggle is fine', (t) => {
  boot(t, { noToggle: true });
  assert.equal(typeof window.goneTheme.toggle, 'function');
});

test('waits for DOMContentLoaded while the document is loading', (t) => {
  const b = boot(t, { readyState: 'loading' });
  assert.equal(b.btn.hidden, true);
  b.env.document.dispatch('DOMContentLoaded');
  assert.equal(b.btn.hidden, false);
});

test('reveals every security warning over plain HTTP only', (t) => {
  const plain = boot(t, { url: 'http://gone.test/' });
  assert.ok(plain.warnings.every((w) => w.hidden === false));
  assert.equal(plain.logs.warn.length, 1);
  const secure = boot(t);
  assert.ok(secure.warnings.every((w) => w.hidden === true));
  assert.deepEqual(secure.logs.warn, []);
});

test('trusts isSecureContext when the browser reports it', (t) => {
  t.after(() => { delete globalThis.isSecureContext; });
  globalThis.isSecureContext = true;
  const local = boot(t, { url: 'http://localhost:8080/' });
  assert.ok(local.warnings.every((w) => w.hidden === true));
  globalThis.isSecureContext = false;
  const lan = boot(t, { url: 'https://gone.test/' });
  assert.ok(lan.warnings.every((w) => w.hidden === false));
});
