'use strict';

// Minimal browser environment for unit-testing the classic (non-module)
// scripts in web/js with node:test. Scripts are evaluated in this realm via
// vm.runInThisContext so typed arrays and errors compare normally and V8
// coverage is attributed to the original files.

const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const JS_DIR = path.join(__dirname, '..', '..', 'web', 'js');

class FakeClassList {
  constructor() { this.set = new Set(); }
  add(...c) { c.forEach((x) => this.set.add(x)); }
  remove(...c) { c.forEach((x) => this.set.delete(x)); }
  contains(c) { return this.set.has(c); }
  toggle(c, force) {
    const on = force === undefined ? !this.set.has(c) : Boolean(force);
    if (on) this.set.add(c); else this.set.delete(c);
    return on;
  }
}

class FakeText {
  constructor(text) { this.nodeType = 3; this.textContent = String(text); this.parentNode = null; }
}

class FakeElement {
  constructor(tag) {
    this.tagName = tag.toUpperCase();
    this.nodeType = 1;
    this.id = '';
    this.children = [];
    this.parentNode = null;
    this.classList = new FakeClassList();
    this.attributes = new Map();
    this.listeners = new Map();
    this.style = {};
    this.dataset = {};
    this.hidden = false;
    this.disabled = false;
    this.readOnly = false;
    this.value = '';
    this.scrollHeight = 0;
    this._text = '';
  }

  get className() { return Array.from(this.classList.set).join(' '); }
  set className(v) { this.classList.set = new Set(String(v).split(/\s+/).filter(Boolean)); }

  get textContent() { return this._text + this.children.map((c) => c.textContent).join(''); }
  set textContent(v) { this._detachAll(); this._text = String(v); }

  get childNodes() { return this.children; }

  _detachAll() { this.children.forEach((c) => { c.parentNode = null; }); this.children = []; }

  setAttribute(k, v) { this.attributes.set(k, String(v)); }
  getAttribute(k) { return this.attributes.has(k) ? this.attributes.get(k) : null; }
  hasAttribute(k) { return this.attributes.has(k); }
  removeAttribute(k) { this.attributes.delete(k); if (k === 'value') this.value = undefined; }
  toggleAttribute(k, force) {
    const on = force === undefined ? !this.attributes.has(k) : Boolean(force);
    if (on) this.attributes.set(k, ''); else this.attributes.delete(k);
    return on;
  }

  appendChild(node) {
    if (node.parentNode) node.parentNode._removeChild(node);
    node.parentNode = this;
    this.children.push(node);
    return node;
  }
  // append and replaceChildren accept strings, which become text nodes.
  append(...nodes) { nodes.forEach((n) => this.appendChild(typeof n === 'string' ? new FakeText(n) : n)); }
  replaceChildren(...nodes) { this._detachAll(); this._text = ''; this.append(...nodes); }
  _removeChild(node) { this.children = this.children.filter((c) => c !== node); node.parentNode = null; }
  remove() { if (this.parentNode) this.parentNode._removeChild(this); }
  replaceWith(node) {
    const parent = this.parentNode;
    if (!parent) return;
    if (node.parentNode) node.parentNode._removeChild(node);
    parent.children[parent.children.indexOf(this)] = node;
    node.parentNode = parent;
    this.parentNode = null;
  }
  contains(node) {
    for (let n = node; n; n = n.parentNode) if (n === this) return true;
    return false;
  }

  addEventListener(type, fn) {
    if (!this.listeners.has(type)) this.listeners.set(type, []);
    this.listeners.get(type).push(fn);
  }
  // dispatch runs listeners for type and resolves once async ones settle.
  dispatch(type, props) {
    const ev = Object.assign({ type: type, target: this, defaultPrevented: false, preventDefault() { this.defaultPrevented = true; } }, props);
    const results = (this.listeners.get(type) || []).map((fn) => fn.call(this, ev));
    ev.settled = Promise.all(results);
    return ev;
  }
  click() { return this.dispatch('click'); }
  focus() { globalThis.document.activeElement = this; }
  select() { this.selected = true; }

  matches(sel) { return matches(this, sel); }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
  querySelectorAll(sel) {
    const out = [];
    const walk = (n) => n.children.forEach((c) => { if (c.nodeType === 1) { if (matches(c, sel)) out.push(c); walk(c); } });
    walk(this);
    return out;
  }
  closest(sel) {
    for (let n = this; n && n.nodeType === 1; n = n.parentNode) if (matches(n, sel)) return n;
    return null;
  }
}

// matches supports "tag", "#id", ".class" and 'tag[attr="v"]' selectors.
function matches(node, sel) {
  const m = /^([a-z0-9]*)(?:#([\w-]+))?(?:\.([\w-]+))?(?:\[(\w+)="([^"]*)"\])?$/i.exec(sel);
  if (!m) throw new Error('unsupported selector ' + sel);
  if (m[1] && node.tagName !== m[1].toUpperCase()) return false;
  if (m[2] && node.id !== m[2]) return false;
  if (m[3] && !node.classList.contains(m[3])) return false;
  if (m[4]) {
    const v = m[4] in node && typeof node[m[4]] === 'string' ? node[m[4]] : node.getAttribute(m[4]);
    if (v !== m[5]) return false;
  }
  return true;
}

class FakeDocument {
  constructor() {
    this.body = new FakeElement('body');
    this.activeElement = null;
  }
  createElement(tag) { return new FakeElement(tag); }
  createElementNS(ns, tag) { const el = new FakeElement(tag); el.namespaceURI = ns; return el; }
  querySelector(sel) { return this.body.querySelector(sel); }
  createTextNode(text) { return new FakeText(text); }
  getElementById(id) {
    let found = null;
    const walk = (n) => n.children.forEach((c) => { if (!found && c.nodeType === 1) { if (c.id === id) found = c; else walk(c); } });
    walk(this.body);
    return found;
  }
}

// h builds an element tree: h('div', {id:'x', className:'card'}, [children]).
function h(tag, props, children) {
  const node = new FakeElement(tag);
  Object.entries(props || {}).forEach(([k, v]) => {
    if (k === 'attrs') Object.entries(v).forEach(([a, b]) => node.setAttribute(a, b));
    else if (k === 'dataset') Object.assign(node.dataset, v);
    else node[k] = v;
  });
  (children || []).forEach((c) => node.appendChild(c));
  return node;
}

function define(name, value) {
  Object.defineProperty(globalThis, name, { value: value, writable: true, configurable: true });
}

const GONE_GLOBALS = ['goneUtil', 'goneCrypto', 'goneFileMeta', 'goneEnvelope', 'goneConsumeApi', 'goneConsumeView',
  'goneFileSelection', 'goneSizeMeter', 'goneUpload', 'goneResultPanel', 'goneIcons'];

// reset installs a fresh fake browser at url and removes loaded gone modules.
// Returns {document, alerts, clipboard, logs, storage}.
function reset(url, opts) {
  const o = opts || {};
  GONE_GLOBALS.forEach((g) => { delete globalThis[g]; });
  const env = { alerts: [], clipboard: { text: '', fail: false }, storage: o.storage || {}, windowListeners: {} };
  env.document = new FakeDocument();
  define('window', globalThis);
  define('document', env.document);
  define('location', new URL(url || 'https://gone.test/'));
  define('alert', (m) => env.alerts.push(m));
  define('requestAnimationFrame', (fn) => { fn(); return 1; });
  define('navigator', {
    clipboard: {
      writeText: async (t) => {
        if (env.clipboard.fail) throw new Error('denied');
        env.clipboard.text = t;
      }
    }
  });
  define('localStorage', o.storageThrows
    ? new Proxy({}, { get() { throw new Error('blocked'); } })
    : {
      getItem: (k) => (k in env.storage ? env.storage[k] : null),
      setItem: (k, v) => { env.storage[k] = String(v); }
    });
  delete globalThis.matchMedia;
  define('addEventListener', (type, fn) => { env.windowListeners[type] = fn; });
  return env;
}

// load evaluates web/js/<name>.js in this realm.
function load(...names) {
  names.forEach((name) => {
    const file = path.join(JS_DIR, name + '.js');
    vm.runInThisContext(fs.readFileSync(file, 'utf8'), { filename: file });
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

module.exports = { FakeElement, h, reset, load, fastUtil, captureConsole, waitFor, matches };
