/* global __dirname, setImmediate, document */
'use strict';

// Minimal browser environment for unit-testing the classic (non-module)
// scripts in web/js with node:test. Scripts are evaluated in this realm via
// vm.runInThisContext so typed arrays and errors compare normally and V8
// coverage is attributed to the original files.

const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { FakeElement, FakeDocument, h, matches } = require('./harness-dom');

const JS_DIR = path.join(__dirname, '..', '..', 'web', 'js');

function define(name, value) {
  Object.defineProperty(globalThis, name, { value: value, writable: true, configurable: true });
}

const GONE_GLOBALS = ['goneUtil', 'goneCrypto', 'goneCryptoEncoding', 'goneCryptoCore', 'goneCryptoCipher',
  'goneCryptoV2Inputs', 'goneCryptoV2Key', 'goneCryptoV2', 'goneFileMeta', 'goneEnvelope', 'goneConsumeApi',
  'goneConsumeApiErrors', 'goneConsumeApiEndpoint', 'goneConsumeApiRead', 'goneConsumeApiFetch',
  'goneConsumeApiCrypto', 'goneConsumeApiAck', 'goneConsumeView', 'goneConsumeViewBase',
  'goneConsumeViewStatus', 'goneConsumeViewPassphrase', 'goneConsumeViewMask', 'goneConsumeViewMessages', 'goneConsumeViewFiles',
  'goneConsumeViewFinal', 'goneFileSelection', 'goneSizeMeter', 'goneUpload', 'goneResultPanel', 'goneIcons',
  'goneTheme', 'goneManageApi', 'goneManageView', 'goneWordlist', 'gonePassgen', 'gonePassphraseField',
  'goneSubmitDom', 'goneSubmitState', 'goneSubmitUi', 'goneSubmitRun', 'goneSubmitPreview', 'goneConsumeOpener',
  'goneCryptoV3', 'goneRequestStore', 'goneRequestApi', 'goneRequestPoll', 'goneRequestList',
  'goneRequestDetailView', 'goneSubmitTarget', 'goneQr', 'goneResultQr', 'goneI18n', 'goneLangPicker'];

const MESSAGES_FILE = path.join(__dirname, '..', '..', 'web', 'messages', 'en.json');

// wireMessage converts one inlang catalog value to the form the server sends
// the browser: a pattern string, or {$plural, <category>: pattern}.
function wireMessage(value) {
  if (typeof value === 'string') return value;
  const v = value[0];
  const selector = v.selectors[0];
  const decl = v.declarations.find((d) => d.startsWith('local ' + selector));
  const out = { $plural: decl.split('=')[1].split(':')[0].trim() };
  Object.entries(v.match).forEach(([k, pattern]) => {
    const cat = k.split('=')[1];
    out[cat === '*' ? 'other' : cat] = pattern;
  });
  return out;
}

// englishMessages returns the whole English catalog in wire form.
function englishMessages() {
  const raw = JSON.parse(fs.readFileSync(MESSAGES_FILE, 'utf8'));
  const out = {};
  Object.entries(raw).forEach(([k, v]) => { if (k !== '$schema') out[k] = wireMessage(v); });
  return out;
}

const EN_MESSAGES = englishMessages();

const MODULE_DEPS = {
  crypto: ['cryptoEncoding', 'cryptoCore', 'cryptoCipher', 'cryptoV2Inputs', 'cryptoV2Key', 'cryptoV2'],
  consumeApi: ['consumeApiErrors', 'consumeApiEndpoint', 'consumeApiRead', 'consumeApiFetch', 'consumeApiCrypto', 'consumeApiAck'],
  consumeView: ['consumeViewBase', 'consumeViewPassphrase', 'consumeViewStatus', 'consumeViewMask', 'consumeViewMessages', 'consumeViewFiles', 'consumeViewFinal'],
  submit: ['submitDom', 'submitState', 'submitUi', 'submitRun', 'submitPreview'],
  consume: ['consumeOpener']
};

// reset installs a fresh fake browser at url and removes loaded gone modules.
// opts: {storage, storageThrows, readyState, indexedDB}.
// Returns {document, alerts, clipboard, storage, selection, windowListeners}.
function reset(url, opts) {
  const o = opts || {};
  GONE_GLOBALS.forEach((g) => { delete globalThis[g]; });
  const env = baseEnv(o);
  env.document = new FakeDocument(o.readyState);
  defineBrowserGlobals(url, env, o);
  return env;
}

function baseEnv(opts) {
  return {
    alerts: [],
    clipboard: { text: '', fail: false },
    storage: opts.storage || {},
    windowListeners: {},
    selection: { ranges: [] }
  };
}

function defineBrowserGlobals(url, env, opts) {
  define('window', globalThis);
  define('document', env.document);
  define('location', new URL(url || 'https://gone.test/'));
  define('alert', (m) => env.alerts.push(m));
  define('requestAnimationFrame', (fn) => { fn(); return 1; });
  defineNavigator(env);
  define('localStorage', storageFor(env, opts));
  if (opts.indexedDB) define('indexedDB', opts.indexedDB);
  else delete globalThis.indexedDB;
  define('getSelection', () => selectionFor(env));
  delete globalThis.matchMedia;
  define('addEventListener', (type, fn) => { env.windowListeners[type] = fn; });
}

function defineNavigator(env) {
  define('navigator', {
    clipboard: {
      writeText: async (t) => {
        if (env.clipboard.fail) throw new Error('denied');
        env.clipboard.text = t;
      }
    }
  });
}

function storageFor(env, opts) {
  if (opts.storageThrows) return new Proxy({}, { get() { throw new Error('blocked'); } });
  return {
    getItem: (k) => (k in env.storage ? env.storage[k] : null),
    setItem: (k, v) => { env.storage[k] = String(v); },
    removeItem: (k) => { delete env.storage[k]; }
  };
}

function selectionFor(env) {
  return {
    removeAllRanges() { env.selection.ranges = []; },
    addRange(r) { env.selection.ranges.push(r); }
  };
}

// loadI18n loads i18n.js with the English catalog (or the given page data)
// the way the server inlines it, then removes the data block again so tests
// see only the DOM they built.
function loadI18n(data) {
  const block = document.createElement('script');
  block.id = 'gone-i18n';
  block.textContent = JSON.stringify(data || { locale: 'en', dir: 'ltr', locales: [{ tag: 'en', name: 'English', dir: 'ltr' }], catalogs: {}, messages: EN_MESSAGES });
  document.body.appendChild(block);
  runFile('i18n');
  block.remove();
}

function runFile(name) {
  const file = path.join(JS_DIR, name + '.js');
  vm.runInThisContext(fs.readFileSync(file, 'utf8'), { filename: file });
}

// loadRaw evaluates web/js/<name>.js alone, without loading i18n.js first,
// to test how a module behaves when goneI18n is missing.
function loadRaw(name) {
  runFile(name);
}

// load evaluates web/js/<name>.js in this realm, after i18n.js unless that
// is already loaded (call loadI18n first to give it other page data).
function load(...names) {
  names.forEach((name) => {
    if (!globalThis.goneI18n && name !== 'i18n') loadI18n();
    (MODULE_DEPS[name] || []).forEach((dep) => load(dep));
    if (name === 'i18n' && globalThis.goneI18n) return;
    runFile(name);
  });
}

// fastUtil swaps goneUtil for a copy whose sleep resolves immediately and
// records requested delays; load dependants afterwards.
function fastUtil() {
  const delays = [];
  globalThis.goneUtil = Object.assign({}, globalThis.goneUtil, {
    sleep: (ms) => { delays.push(ms); return Promise.resolve(); }
  });
  return delays;
}

// captureConsole silences and records console output for the test's duration.
function captureConsole(t) {
  const logs = { log: [], warn: [], error: [] };
  Object.keys(logs).forEach((k) => {
    t.mock.method(console, k, (...args) => { logs[k].push(args.map(String).join(' ')); });
  });
  return logs;
}

// waitFor polls cond until it is truthy or the timeout expires.
async function waitFor(cond, ms) {
  const end = Date.now() + (ms || 2000);
  while (Date.now() < end) {
    if (cond()) return;
    await new Promise((r) => setImmediate(r));
  }
  throw new Error('waitFor timed out');
}

module.exports = {
  loadI18n, loadRaw, EN_MESSAGES, FakeElement, h, reset, load, fastUtil, captureConsole, waitFor, matches };
