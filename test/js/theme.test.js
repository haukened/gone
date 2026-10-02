'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h, captureConsole } = require('./harness');

const KEY = 'gone.theme';

// boot builds the theme switch (and optional warning) then loads theme.js.
// mq.dark sets the system preference; mq.throws makes matchMedia throw.
function boot(t, opts) {
  const o = opts || {};
  const env = reset(o.url || 'https://gone.test/', { storage: o.storage });
  const checkbox = h('input', { id: 'theme-switch' });
  const desc = h('span', { id: 'theme-switch-desc' });
  const warning = h('section', { className: 'security-warning', hidden: true });
  env.document.body.append(...[checkbox, o.noDesc ? null : desc, o.noWarning ? null : warning].filter(Boolean));
  const mq = { matches: Boolean(o.dark), listeners: [], addEventListener(type, fn) { this.listeners.push(fn); } };
  if (o.mq !== false) {
    globalThis.matchMedia = () => { if (o.mqThrows) throw new Error('nope'); return mq; };
  }
  if (o.storageOverride) globalThis.localStorage = o.storageOverride(env.storage);
  const logs = captureConsole(t);
  load('theme');
  return { env, checkbox, desc, warning, mq, logs };
}

test('does nothing without the switch', (t) => {
  reset();
  const logs = captureConsole(t);
  load('theme');
  assert.deepEqual(logs.log, []);
});

test('uses the stored theme without persisting', (t) => {
  for (const mode of ['dark', 'light']) {
    const b = boot(t, { storage: { [KEY]: mode }, dark: mode !== 'dark' });
    assert.equal(b.checkbox.checked, mode === 'dark');
    assert.equal(b.checkbox.getAttribute('aria-pressed'), String(mode === 'dark'));
    assert.match(b.desc.textContent, new RegExp('Currently ' + mode));
    assert.equal(b.mq.listeners.length, 0);
    assert.deepEqual(b.logs.log, ['Gone theme module loaded']);
  }
});

test('falls back to the system preference and persists it', (t) => {
  const dark = boot(t, { dark: true });
  assert.equal(dark.checkbox.checked, true);
  assert.equal(dark.env.storage[KEY], 'dark');
  const light = boot(t, { storage: { [KEY]: 'bogus' }, dark: false });
  assert.equal(light.checkbox.checked, false);
  assert.equal(light.env.storage[KEY], 'light');
  const none = boot(t, { mq: false });
  assert.equal(none.env.storage[KEY], 'light');
  const broken = boot(t, { mq: true, mqThrows: true, storage: { [KEY]: 'x' } });
  assert.equal(broken.env.storage[KEY], 'light');
});

test('toggling the switch persists the choice', (t) => {
  const b = boot(t, { storage: { [KEY]: 'light' }, noDesc: true });
  b.checkbox.checked = true;
  b.checkbox.dispatch('change');
  assert.equal(b.env.storage[KEY], 'dark');
  b.checkbox.checked = false;
  b.checkbox.dispatch('change');
  assert.equal(b.env.storage[KEY], 'light');
});

test('follows system changes only while no theme is stored', (t) => {
  const b = boot(t, { dark: false, storageOverride: (store) => ({
    getItem: (k) => (k in store ? store[k] : null),
    setItem: (k, v) => { if (b && b.ready) store[k] = v; }
  }) });
  b.ready = true;
  assert.equal(b.mq.listeners.length, 1);
  b.mq.listeners[0]({ matches: true });
  assert.equal(b.checkbox.checked, true);
  assert.equal(b.env.storage[KEY], 'dark');
  b.mq.listeners[0]({ matches: false });
  assert.equal(b.checkbox.checked, true);
});

test('storage write failures are tolerated', (t) => {
  const failingSet = (store) => ({
    getItem: (k) => (k in store ? store[k] : null),
    setItem: () => { throw new Error('quota'); }
  });
  const b = boot(t, { storageOverride: failingSet });
  // initial persistence failed, so the theme is left untouched
  assert.equal(b.checkbox.getAttribute('aria-pressed'), null);
  b.checkbox.checked = true;
  b.checkbox.dispatch('change');
  assert.equal(b.checkbox.checked, true);
});

test('storage read failures during initial load are tolerated', (t) => {
  let reads = 0;
  const b = boot(t, { storageOverride: () => ({
    getItem: () => { if (reads++ > 0) throw new Error('blocked'); return null; },
    setItem: () => {}
  }) });
  assert.equal(b.checkbox.getAttribute('aria-pressed'), null);
  assert.deepEqual(b.logs.log, ['Gone theme module loaded']);
});

test('shows the insecure-context warning over plain HTTP', (t) => {
  const b = boot(t, { url: 'http://gone.test/', storage: { [KEY]: 'light' } });
  assert.equal(b.warning.hidden, false);
  assert.equal(b.warning.getAttribute('aria-hidden'), 'false');
  assert.match(b.logs.warn[0], /insecure context/);
  const n = boot(t, { url: 'http://gone.test/', storage: { [KEY]: 'light' }, noWarning: true });
  assert.equal(n.logs.warn.length, 1);
  const s = boot(t, { storage: { [KEY]: 'light' } });
  assert.equal(s.warning.hidden, true);
});

test('a failing location lookup does not break loading', (t) => {
  const env = reset('https://gone.test/', { storage: { [KEY]: 'light' } });
  env.document.body.appendChild(h('input', { id: 'theme-switch' }));
  Object.defineProperty(globalThis, 'location', { get() { throw new Error('denied'); }, configurable: true });
  const logs = captureConsole(t);
  load('theme');
  assert.deepEqual(logs.log, ['Gone theme module loaded']);
});
