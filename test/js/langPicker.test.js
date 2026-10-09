/* global document */
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, loadI18n, loadRaw, h, waitFor, EN_MESSAGES } = require('./harness');

const LOCALES = [{ tag: 'en', name: 'English', dir: 'ltr' }, { tag: 'pt-BR', name: 'Português (Brasil)', dir: 'ltr' }];

// picker builds the header picker markup and loads the module with the given
// page locales.
function picker(t, locales, opts) {
  const o = opts || {};
  reset('https://gone.test/', { readyState: o.readyState });
  const select = h('select', { id: 'lang-select' }, [h('option', { value: 'en' }), h('option', { value: 'pt-BR' })]);
  const code = h('span', { className: 'lang-code', textContent: 'EN' });
  const wrap = h('label', { className: 'lang-pick', hidden: true }, [code, select]);
  const status = h('span', { id: 'lang-status' });
  document.body.append(wrap, status);
  loadI18n({ locale: 'en', dir: 'ltr', locales: locales, catalogs: { 'pt-BR': '/i18n/pt-BR.json?v=1' }, messages: EN_MESSAGES });
  load('langPicker');
  return { select, code, wrap, status };
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
  assert.equal(p.select.value, 'en');
  assert.equal(p.code.textContent, 'EN');
  api.wire();
});

test('stays hidden with one language or without its markup', () => {
  const p = picker(null, [LOCALES[0]]);
  assert.equal(p.wrap.hidden, true);
  reset();
  load('langPicker');
  assert.doesNotThrow(() => window.goneLangPicker.wire());
  reset();
  document.body.append(h('select', { id: 'lang-select' }));
  load('langPicker');
  assert.doesNotThrow(() => window.goneLangPicker.wire());
});

test('waits for the DOM when loaded early', () => {
  const p = picker(null, LOCALES, { readyState: 'loading' });
  assert.equal(p.wrap.hidden, true);
  document.dispatch('DOMContentLoaded');
  assert.equal(p.wrap.hidden, false);
});

test('choosing a language switches in place; a failure is undone and announced', async (t) => {
  const p = picker(t, LOCALES);
  let fail = false;
  t.mock.method(globalThis, 'fetch', async () => {
    if (fail) throw new Error('offline');
    return { ok: true, status: 200, json: async () => Object.assign({}, EN_MESSAGES, { 'js.lang.failed': 'Não foi possível trocar o idioma.' }) };
  });
  p.select.value = 'pt-BR';
  p.select.dispatch('change');
  assert.equal(p.select.getAttribute('aria-busy'), 'true');
  await waitFor(() => window.goneI18n.locale === 'pt-BR');
  await waitFor(() => !p.select.hasAttribute('aria-busy'));
  assert.equal(p.code.textContent, 'PT');
  assert.equal(p.status.textContent, '');
  assert.equal(document.documentElement.lang, 'pt-BR');

  fail = true;
  p.select.value = 'en';
  p.select.dispatch('change');
  await waitFor(() => p.status.textContent !== '');
  assert.equal(p.select.value, 'pt-BR');
  assert.equal(p.code.textContent, 'PT');
  assert.equal(p.status.textContent, 'Não foi possível trocar o idioma.');
});
