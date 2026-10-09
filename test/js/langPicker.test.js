/* global document */
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, loadI18n, loadRaw, h, waitFor, EN_MESSAGES } = require('./harness');

const LOCALES = [
  { tag: 'en', name: 'English', dir: 'ltr' },
  { tag: 'es', name: 'Español', dir: 'ltr' },
  { tag: 'pt-BR', name: 'Português (Brasil)', dir: 'ltr' }
];

// picker builds the header picker markup and loads the module with the given
// page locales.
function picker(t, locales, opts) {
  const o = opts || {};
  reset('https://gone.test/', { readyState: o.readyState });
  const options = LOCALES.map((l) => h('button', { className: 'lang-option', dataset: { lang: l.tag }, textContent: l.name }));
  const code = h('span', { className: 'lang-code', textContent: 'EN' });
  const button = h('button', { id: 'lang-button', attrs: { 'aria-expanded': 'false' } }, [code]);
  const menu = h('ul', { id: 'lang-menu', hidden: true }, options.map((b) => h('li', {}, [b])));
  const wrap = h('div', { className: 'lang-pick', hidden: true }, [button, menu]);
  const status = h('span', { id: 'lang-status' });
  const outside = h('p');
  document.body.append(wrap, status, outside);
  loadI18n({ locale: 'en', dir: 'ltr', locales: locales, catalogs: { es: '/i18n/es.json?v=1', 'pt-BR': '/i18n/pt-BR.json?v=1' }, messages: EN_MESSAGES });
  load('langPicker');
  return { button, menu, code, wrap, status, outside, options };
}

function current(p) {
  return p.options.filter((o) => o.getAttribute('aria-current') === 'true').map((o) => o.dataset.lang);
}

function key(target, k) {
  return target.dispatch('keydown', { key: k });
}

test('needs goneI18n and loads once', () => {
  reset();
  loadRaw('langPicker');
  assert.equal(window.goneLangPicker, undefined);
  const p = picker(null, LOCALES);
  const api = window.goneLangPicker;
  load('langPicker');
  assert.equal(window.goneLangPicker, api);
  assert.equal(p.wrap.hidden, false);
  assert.equal(p.code.textContent, 'EN');
  assert.deepEqual(current(p), ['en']);
  api.wire();
});

test('stays hidden with one language or without its markup', () => {
  const p = picker(null, [LOCALES[0]]);
  assert.equal(p.wrap.hidden, true);
  reset();
  load('langPicker');
  assert.doesNotThrow(() => window.goneLangPicker.wire());
  reset();
  document.body.append(h('div', { className: 'lang-pick', hidden: true }, [h('button', { id: 'lang-button' })]));
  load('langPicker');
  assert.doesNotThrow(() => window.goneLangPicker.wire());
  assert.equal(document.querySelector('.lang-pick').hidden, true);
});

test('waits for the DOM when loaded early', () => {
  const p = picker(null, LOCALES, { readyState: 'loading' });
  assert.equal(p.wrap.hidden, true);
  document.dispatch('DOMContentLoaded');
  assert.equal(p.wrap.hidden, false);
});

test('the pill opens and closes the list, focusing the current language', () => {
  const p = picker(null, LOCALES);
  p.button.click();
  assert.equal(p.menu.hidden, false);
  assert.equal(p.button.getAttribute('aria-expanded'), 'true');
  assert.equal(document.activeElement, p.options[0]);
  p.button.click();
  assert.equal(p.menu.hidden, true);
  assert.equal(p.button.getAttribute('aria-expanded'), 'false');

  // With no option for the current language, the first one takes focus.
  p.options[0].dataset.lang = 'xx';
  p.button.click();
  assert.equal(document.activeElement, p.options[0]);
});

test('keys move through the list; Escape closes it and returns to the pill', () => {
  const p = picker(null, LOCALES);
  assert.equal(key(p.button, 'Enter').defaultPrevented, false);
  assert.equal(p.menu.hidden, true);
  assert.equal(key(p.button, 'ArrowDown').defaultPrevented, true);
  assert.equal(p.menu.hidden, false);
  assert.equal(document.activeElement, p.options[0]);
  key(p.menu, 'ArrowDown');
  assert.equal(document.activeElement, p.options[1]);
  key(p.menu, 'End');
  assert.equal(document.activeElement, p.options[2]);
  key(p.menu, 'ArrowDown');
  assert.equal(document.activeElement, p.options[0]);
  key(p.menu, 'ArrowUp');
  assert.equal(document.activeElement, p.options[2]);
  key(p.menu, 'Home');
  assert.equal(document.activeElement, p.options[0]);
  assert.equal(key(p.menu, 'a').defaultPrevented, false);
  assert.equal(key(p.menu, 'Escape').defaultPrevented, true);
  assert.equal(p.menu.hidden, true);
  assert.equal(document.activeElement, p.button);
  key(p.button, 'ArrowUp');
  assert.equal(p.menu.hidden, false);
});

test('clicking or tabbing away closes the list', () => {
  const p = picker(null, LOCALES);
  p.button.click();
  document.dispatch('click', { target: p.options[1] });
  assert.equal(p.menu.hidden, false);
  document.dispatch('click', { target: p.outside });
  assert.equal(p.menu.hidden, true);
  document.dispatch('click', { target: p.outside });
  assert.equal(p.menu.hidden, true);

  p.button.click();
  p.wrap.dispatch('focusout', { relatedTarget: p.options[1] });
  p.wrap.dispatch('focusout', { relatedTarget: null });
  assert.equal(p.menu.hidden, false);
  p.wrap.dispatch('focusout', { relatedTarget: p.outside });
  assert.equal(p.menu.hidden, true);
});

test('choosing a language switches in place; a failure is announced', async (t) => {
  const p = picker(t, LOCALES);
  let fail = false;
  t.mock.method(globalThis, 'fetch', async () => {
    if (fail) throw new Error('offline');
    return { ok: true, status: 200, json: async () => Object.assign({}, EN_MESSAGES, { 'js.lang.failed': 'Não foi possível trocar o idioma.' }) };
  });
  p.button.click();
  p.menu.dispatch('click', { target: p.menu });
  assert.equal(p.menu.hidden, false);
  p.menu.dispatch('click', { target: p.options[2] });
  assert.equal(p.menu.hidden, true);
  assert.equal(document.activeElement, p.button);
  assert.equal(p.button.getAttribute('aria-busy'), 'true');
  await waitFor(() => window.goneI18n.locale === 'pt-BR');
  await waitFor(() => !p.button.hasAttribute('aria-busy'));
  assert.equal(p.code.textContent, 'PT');
  assert.deepEqual(current(p), ['pt-BR']);
  assert.equal(p.status.textContent, '');
  assert.equal(document.documentElement.lang, 'pt-BR');

  // The language already in use just closes the list.
  p.button.click();
  await p.menu.dispatch('click', { target: p.options[2] }).settled;
  assert.equal(p.menu.hidden, true);
  assert.equal(p.button.hasAttribute('aria-busy'), false);

  fail = true;
  p.button.click();
  p.menu.dispatch('click', { target: p.options[1] });
  await waitFor(() => p.status.textContent !== '');
  assert.equal(window.goneI18n.locale, 'pt-BR');
  assert.equal(p.code.textContent, 'PT');
  assert.deepEqual(current(p), ['pt-BR']);
  assert.equal(p.status.textContent, 'Não foi possível trocar o idioma.');
});
