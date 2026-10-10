/* global document */
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, loadI18n, h, EN_MESSAGES } = require('./harness');

const ES = {
  'js.greet': 'Hola {name}',
  'js.files': { $plural: 'count', one: 'un archivo', many: '{count} de archivos', other: '{count} archivos' },
  'page.rich': 'Lee el {#link}protocolo{/link} y más.',
  'page.time': 'Hasta {#time}{/time}',
  'page.label': 'Etiqueta',
  'common.size.b': '{n} B',
  'common.size.kb': '{n} KB',
  'common.duration.long.hours': { $plural: 'n', one: '{n} hora', other: '{n} horas' },
};

const PAGE = {
  locale: 'en',
  dir: 'ltr',
  locales: [{ tag: 'en', name: 'English', dir: 'ltr' }, { tag: 'es', name: 'Español', dir: 'ltr' }, { tag: 'ar', name: 'العربية', dir: 'rtl' }],
  catalogs: { es: '/i18n/es.json?v=1', ar: '/i18n/ar.json?v=1' },
  messages: Object.assign({}, EN_MESSAGES, {
    'js.greet': 'Hello {name}',
    'js.files': { $plural: 'count', one: 'a file', other: '{count} files' },
    'page.rich': 'Read the {#link}protocol{/link} now.',
    'page.time': 'Until {#time}{/time}',
    'page.label': 'Label',
    'js.broken': { $plural: 'count' },
    'js.odd': 42,
  }),
};

// boot loads i18n.js with PAGE, or the default English page data.
function boot(data, url) {
  reset(url);
  loadI18n(data === undefined ? PAGE : data);
  return window.goneI18n;
}

// stubFetch answers catalog requests from bodies (url -> {status, json}).
function stubFetch(t, bodies) {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, init) => {
    calls.push({ url, init });
    const b = bodies[url];
    if (!b) throw new Error('offline');
    return { ok: b.status === 200, status: b.status, json: async () => b.json };
  });
  return calls;
}

test('loads once and reads the page data', () => {
  const i = boot();
  assert.equal(i.locale, 'en');
  assert.deepEqual(i.locales().map((l) => l.tag), ['en', 'es', 'ar']);
  load('i18n');
  assert.equal(window.goneI18n, i);
  assert.ok(Object.isFrozen(i));
});

test('missing or malformed page data falls back to the document language', () => {
  reset();
  document.documentElement.lang = 'fr';
  load('i18n');
  assert.equal(window.goneI18n.locale, 'fr');
  assert.deepEqual(window.goneI18n.locales(), []);
  assert.equal(window.goneI18n.t('js.greet'), 'js.greet');

  reset();
  const block = h('script', { id: 'gone-i18n', textContent: '{oops' });
  document.body.appendChild(block);
  document.documentElement.lang = '';
  load('i18n');
  assert.equal(window.goneI18n.locale, 'en');
  block.remove();

  reset();
  document.body.appendChild(h('script', { id: 'gone-i18n', textContent: 'null' }));
  load('i18n');
  assert.equal(window.goneI18n.locale, 'en');
});

test('t renders placeholders, plurals and unknown keys', () => {
  const i = boot();
  assert.equal(i.t('js.greet', { name: 'Ana' }), 'Hello Ana');
  assert.equal(i.t('js.greet'), 'Hello ');
  assert.equal(i.t('js.files', { count: 1 }), 'a file');
  assert.equal(i.t('js.files', { count: 3 }), '3 files');
  assert.equal(i.t('js.files', { count: { num: 1, digits: 0 } }), 'a file');
  assert.equal(i.t('js.files', { count: 'x' }), 'x files');
  assert.equal(i.t('js.files'), ' files');
  assert.equal(i.t('js.broken', { count: 1 }), 'js.broken');
  assert.equal(i.t('js.odd'), 'js.odd');
  assert.equal(i.t('page.rich'), 'Read the protocol now.');
  assert.equal(i.t('no.such.key'), 'no.such.key');
});

test('format renders every argument kind in the current locale', () => {
  const i = boot();
  const cases = [
    [1234.5, '1,234.5'],
    ['text', 'text'],
    [{ num: 2, digits: 1 }, '2.0'],
    [{ num: 2 }, '2'],
    [{ bytes: 512 }, '512 B'],
    [{ bytes: 1536 }, '1.5 KB'],
    [{ bytes: 10 * 1024 * 1024 }, '10.0 MB'],
    [{ bytes: 2 ** 60 }, '1,024.0 PB'],
    [{ seconds: 7200, style: 'short' }, '2h'],
    [{ seconds: 3600 }, '1 hour'],
    [{ seconds: 90 }, '90 seconds'],
    [{ seconds: 0, style: 'short' }, '0s'],
    [{ date: 'not a date' }, ''],
    [{ rel: -2, unit: 'day' }, '2 days ago'],
    [{ compact: 12000 }, '12 thousand'],
    [{ msg: 'js.greet', args: { name: 'Bo' } }, 'Hello Bo'],
    [{ other: 1 }, ''],
    [null, ''],
    [undefined, ''],
    [true, ''],
  ];
  for (const [v, want] of cases) assert.equal(i.format(v), want, JSON.stringify(v));
  const when = '2026-01-02T15:04:00Z';
  assert.equal(i.format({ date: when, style: 'time' }), new Date(when).toLocaleString('en', { timeStyle: 'short' }));
  assert.equal(i.format({ date: when, style: 'date' }), new Date(when).toLocaleString('en', { dateStyle: 'medium' }));
  assert.equal(i.format({ date: when }), new Date(when).toLocaleString('en', { dateStyle: 'medium', timeStyle: 'short' }));
});

test('set, setAttr, value, plain and clear remember what to re-render', () => {
  const i = boot();
  const node = h('span');
  i.set(node, 'js.greet', { name: 'Ana' });
  assert.equal(node.textContent, 'Hello Ana');
  assert.equal(node.getAttribute('data-i18n'), 'js.greet');
  assert.equal(node.getAttribute('data-i18n-args'), '{"name":"Ana"}');
  i.set(node, 'page.label');
  assert.equal(node.hasAttribute('data-i18n-args'), false);

  i.setAttr(node, 'aria-label', 'js.greet', { name: 'Bo' });
  i.setAttr(node, 'title', 'page.label');
  i.setAttr(node, 'aria-label', 'js.files', { count: 2 });
  assert.equal(node.getAttribute('data-i18n-attr'), 'title:page.label;aria-label:js.files');
  assert.equal(node.getAttribute('aria-label'), '2 files');
  assert.equal(node.getAttribute('title'), 'Label');

  const v = h('span');
  i.value(v, { bytes: 2048 });
  assert.equal(v.textContent, '2.0 KB');
  assert.equal(v.getAttribute('data-i18n-value'), '{"bytes":2048}');

  i.plain(node, 'typed by someone');
  assert.equal(node.textContent, 'typed by someone');
  assert.equal(node.hasAttribute('data-i18n'), false);
  i.clear(v);
  assert.equal(v.textContent, '');
  assert.equal(v.hasAttribute('data-i18n-value'), false);

  for (const fn of [i.set, i.setAttr, i.value, i.plain, i.clear, i.restore]) assert.doesNotThrow(() => fn(null, 'x'));
  assert.equal(i.snapshot(null), null);
});

test('snapshot and restore bring back a key or plain text', () => {
  const i = boot();
  const keyed = h('span');
  i.set(keyed, 'js.greet', { name: 'Ana' });
  const snap = i.snapshot(keyed);
  i.set(keyed, 'page.label');
  i.restore(keyed, snap);
  assert.equal(keyed.textContent, 'Hello Ana');
  const plainNode = h('span', { textContent: 'raw' });
  const raw = i.snapshot(plainNode);
  i.set(plainNode, 'page.label');
  i.restore(plainNode, raw);
  assert.equal(plainNode.textContent, 'raw');
  assert.equal(plainNode.hasAttribute('data-i18n'), false);
  i.restore(plainNode, null);
  assert.equal(plainNode.textContent, 'raw');
});

test('setTitle updates the title element when there is one', () => {
  const i = boot();
  i.setTitle('js.greet', { name: 'Ana' });
  assert.equal(document.title, 'Hello Ana');
  const title = h('title');
  document.body.appendChild(title);
  i.setTitle('page.label');
  assert.equal(title.textContent, 'Label');
  assert.equal(title.getAttribute('data-i18n'), 'page.label');
});

// page builds a fragment of a server-rendered page.
function page() {
  const link = h('a', { attrs: { href: '/docs', 'data-i18n-slot': 'link' }, textContent: 'protocol' });
  const time = h('time', { attrs: { 'data-i18n-slot': 'time' }, textContent: 'Jan 2' });
  const nodes = {
    label: h('span', { attrs: { 'data-i18n': 'page.label' }, textContent: 'Label' }),
    rich: h('p', { attrs: { 'data-i18n': 'page.rich' } }, [document.createTextNode('Read the '), link, document.createTextNode(' now.')]),
    until: h('span', { attrs: { 'data-i18n': 'page.time' } }, [document.createTextNode('Until '), time]),
    noSlot: h('p', { attrs: { 'data-i18n': 'page.rich' } }),
    files: h('span', { attrs: { 'data-i18n': 'js.files', 'data-i18n-args': '{"count":2}' }, textContent: '2 files' }),
    badArgs: h('span', { attrs: { 'data-i18n': 'js.greet', 'data-i18n-args': '{bad' } }),
    attr: h('input', { attrs: { 'data-i18n-attr': 'placeholder:page.label;bad;aria-label:js.greet', 'data-i18n-args': '{"name":"Zoe"}' } }),
    size: h('span', { attrs: { 'data-i18n-value': '{"bytes":1536}' }, textContent: '1.5 KB' }),
    badValue: h('span', { attrs: { 'data-i18n-value': '{oops' }, textContent: 'kept' }),
    note: h('p', { attrs: { 'data-i18n-hidden-in': 'en' }, hidden: true }),
  };
  Object.values(nodes).forEach((n) => document.body.appendChild(n));
  return Object.assign(nodes, { link, time });
}

test('setLocale switches the page in place and saves the choice', async (t) => {
  const i = boot(undefined, 'https://gone.test/');
  const p = page();
  const calls = stubFetch(t, { '/i18n/es.json?v=1': { status: 200, json: ES } });
  const seen = [];
  i.onChange((tag) => seen.push(tag));
  assert.equal(await i.setLocale('en'), true);
  assert.equal(calls.length, 0);
  assert.equal(await i.setLocale('es'), true);
  assert.equal(i.locale, 'es');
  assert.deepEqual(seen, ['es']);
  assert.equal(calls[0].init.credentials, 'same-origin');
  assert.equal(document.documentElement.lang, 'es');
  assert.equal(document.documentElement.dir, 'ltr');
  assert.equal(document.cookie, 'gone_lang=es; Path=/; Max-Age=31536000; SameSite=Lax; Secure');

  assert.equal(p.label.textContent, 'Etiqueta');
  assert.equal(p.files.textContent, '2 archivos');
  assert.equal(p.rich.children.length, 3);
  assert.equal(p.rich.children[1], p.link);
  assert.equal(p.link.textContent, 'protocolo');
  assert.equal(p.link.getAttribute('href'), '/docs');
  assert.equal(p.rich.textContent, 'Lee el protocolo y más.');
  assert.equal(p.until.children[1], p.time);
  assert.equal(p.time.textContent, 'Jan 2', 'an empty slot keeps its content');
  assert.equal(p.noSlot.textContent, 'Lee el protocolo y más.');
  assert.equal(p.attr.getAttribute('placeholder'), 'Etiqueta');
  assert.equal(p.attr.getAttribute('aria-label'), 'Hola Zoe');
  assert.equal(p.badArgs.textContent, 'Hola ');
  assert.equal(p.size.textContent, '1,5 KB');
  assert.equal(p.badValue.textContent, 'kept');
  assert.equal(p.note.hidden, false);
  assert.equal(i.t('js.files', { count: 1000000 }), '1.000.000 de archivos');
  assert.equal(i.t('missing.in.es'), 'missing.in.es');

  // Switching back fetches the full English catalog once, then reuses it.
  stubFetch(t, {});
  assert.equal(await i.setLocale('es'), true);
});

test('setLocale refuses unknown locales and failed fetches, leaving the page as it was', async (t) => {
  const i = boot(undefined, 'http://gone.test/');
  const p = page();
  stubFetch(t, { '/i18n/ar.json?v=1': { status: 500, json: {} } });
  assert.equal(await i.setLocale('xx'), false);
  assert.equal(await i.setLocale('ar'), false);
  assert.equal(await i.setLocale('es'), false);
  assert.equal(i.locale, 'en');
  assert.equal(p.label.textContent, 'Label');
  assert.equal(document.cookie, undefined);
});

test('a right-to-left locale sets dir, and cookies are not Secure on http', async (t) => {
  const i = boot(undefined, 'http://gone.test/');
  stubFetch(t, { '/i18n/ar.json?v=1': { status: 200, json: { 'page.label': 'تسمية' } } });
  assert.equal(await i.setLocale('ar'), true);
  assert.equal(document.documentElement.dir, 'rtl');
  assert.equal(document.cookie, 'gone_lang=ar; Path=/; Max-Age=31536000; SameSite=Lax');
});

test('apply re-renders a subtree, and an unknown key shows itself', () => {
  const i = boot();
  const node = h('span', { attrs: { 'data-i18n': 'no.such' } });
  const root = h('div', {}, [node]);
  i.apply(root);
  assert.equal(node.textContent, 'no.such');
  document.body.appendChild(h('span', { attrs: { 'data-i18n': 'page.label' } }));
  i.apply();
  assert.equal(document.body.children[0].textContent, 'Label');
});
