'use strict';

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
  constructor(text) {
    this.nodeType = 3;
    this.textContent = String(text);
    this.parentNode = null;
  }
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

  append(...nodes) { nodes.forEach((n) => this.appendChild(textNode(n))); }
  replaceChildren(...nodes) { this._detachAll(); this._text = ''; this.append(...nodes); }
  _detachAll() { this.children.forEach((c) => { c.parentNode = null; }); this.children = []; }
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

  removeEventListener(type, fn) {
    this.listeners.set(type, (this.listeners.get(type) || []).filter((f) => f !== fn));
  }

  dispatch(type, props) {
    const ev = eventFor(this, type, props);
    const results = (this.listeners.get(type) || []).map((fn) => fn.call(this, ev));
    ev.settled = Promise.all(results);
    return ev;
  }

  click() { return this.dispatch('click'); }
  focus() { globalThis.document.activeElement = this; }
  select() { this.selected = true; }
  matches(sel) { return matches(this, sel); }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
  querySelectorAll(sel) { return queryAll(this, sel); }
  closest(sel) { return closest(this, sel); }
}

function textNode(node) {
  return typeof node === 'string' ? new FakeText(node) : node;
}

function eventFor(target, type, props) {
  return Object.assign({
    type: type,
    target: target,
    defaultPrevented: false,
    preventDefault() { this.defaultPrevented = true; }
  }, props);
}

function classNameFor(node) {
  return Array.from(node.classList.set).join(' ');
}

function setClassName(node, value) {
  node.classList.set = new Set(String(value).split(/\s+/).filter(Boolean));
}

function textContentFor(node) {
  return node._text + node.children.map((c) => c.textContent).join('');
}

function setTextContent(node, value) {
  node._detachAll();
  node._text = String(value);
}

Object.defineProperties(FakeElement.prototype, {
  className: { get() { return classNameFor(this); }, set(v) { setClassName(this, v); } },
  textContent: { get() { return textContentFor(this); }, set(v) { setTextContent(this, v); } },
  childNodes: { get() { return this.children; } }
});

class FakeDocument {
  constructor(readyState) {
    this.documentElement = new FakeElement('html');
    this.body = new FakeElement('body');
    this.activeElement = null;
    this.readyState = readyState || 'complete';
    this.title = '';
    this.visibilityState = 'visible';
    this.listeners = new Map();
  }

  createElement(tag) { return new FakeElement(tag); }
  createElementNS(ns, tag) { const el = new FakeElement(tag); el.namespaceURI = ns; return el; }
  createRange() { return { selectNodeContents(n) { this.node = n; } }; }
  querySelector(sel) { return this.body.querySelector(sel); }
  querySelectorAll(sel) { return this.body.querySelectorAll(sel); }
  addEventListener(type, fn) { FakeElement.prototype.addEventListener.call(this, type, fn); }
  removeEventListener(type, fn) { FakeElement.prototype.removeEventListener.call(this, type, fn); }
  dispatch(type, props) { return FakeElement.prototype.dispatch.call(this, type, props); }
  createTextNode(text) { return new FakeText(text); }
  getElementById(id) { return findById(this.body, id); }
}

function findById(root, id) {
  for (const child of root.children) {
    if (child.nodeType !== 1) continue;
    if (child.id === id) return child;
    const found = findById(child, id);
    if (found) return found;
  }
  return null;
}

function h(tag, props, children) {
  const node = new FakeElement(tag);
  applyProps(node, props || {});
  (children || []).forEach((c) => node.appendChild(c));
  return node;
}

function applyProps(node, props) {
  Object.entries(props).forEach(([k, v]) => {
    if (k === 'attrs') Object.entries(v).forEach(([a, b]) => node.setAttribute(a, b));
    else if (k === 'dataset') Object.assign(node.dataset, v);
    else node[k] = v;
  });
}

function parseSelector(sel) {
  const m = /^([a-z0-9]*)(?:#([\w-]+))?(?:\.([\w-]+))?(?:\[(\w+)="([^"]*)"\])?$/i.exec(sel);
  if (!m) throw new Error('unsupported selector ' + sel);
  return { tag: m[1], id: m[2], className: m[3], attr: m[4], value: m[5] };
}

function attrValue(node, attr) {
  return attr in node && typeof node[attr] === 'string' ? node[attr] : node.getAttribute(attr);
}

function matches(node, sel) {
  const parsed = parseSelector(sel);
  return tagMatches(node, parsed) && idMatches(node, parsed) && classMatches(node, parsed) && attrMatches(node, parsed);
}

function tagMatches(node, sel) { return !sel.tag || node.tagName === sel.tag.toUpperCase(); }
function idMatches(node, sel) { return !sel.id || node.id === sel.id; }
function classMatches(node, sel) { return !sel.className || node.classList.contains(sel.className); }
function attrMatches(node, sel) { return !sel.attr || attrValue(node, sel.attr) === sel.value; }

function queryAll(root, sel) {
  const out = [];
  walkElements(root, (child) => { if (matches(child, sel)) out.push(child); });
  return out;
}

function walkElements(root, visit) {
  root.children.forEach((child) => {
    if (child.nodeType !== 1) return;
    visit(child);
    walkElements(child, visit);
  });
}

function closest(node, sel) {
  for (let n = node; n && n.nodeType === 1; n = n.parentNode) if (matches(n, sel)) return n;
  return null;
}

module.exports = { FakeElement, FakeDocument, h, matches };
