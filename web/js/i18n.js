'use strict';

// Translations. The server renders each page in the visitor's language and
// marks every translated node with its message key (data-i18n, plus
// data-i18n-args, data-i18n-attr, data-i18n-value). This module renders the
// same messages in the browser, so text set by scripts is translated too and
// the language can change in place, without a reload that would lose a
// share link, a revealed secret, or typed text.
//
// Loaded in <head>, after the #gone-i18n JSON block and before every other
// script, so modules can look up messages as they start. Exposed as
// window.goneI18n.
(function i18nModule() {
  if (window.goneI18n) return;

  const COOKIE = 'gone_lang';
  const TOKEN = /\{([#/]?)([A-Za-z_][A-Za-z0-9_]*)\}/g;
  const DATE_STYLES = {
    datetime: { dateStyle: 'medium', timeStyle: 'short' },
    date: { dateStyle: 'medium' },
    time: { timeStyle: 'short' },
  };
  const DURATION_UNITS = [['days', 86400], ['hours', 3600], ['minutes', 60], ['seconds', 1]];
  const BYTE_UNITS = ['kb', 'mb', 'gb', 'tb', 'pb'];

  const data = readData();
  const catalogs = {};
  const listeners = [];
  let locale = data.locale || document.documentElement.lang || 'en';
  let messages = data.messages || {};

  function readData() {
    const el = document.getElementById('gone-i18n');
    try {
      return (el && JSON.parse(el.textContent)) || {};
    } catch {
      return {};
    }
  }

  // pick returns the pattern for a message: the plural variant for its input
  // argument's CLDR category, falling back to "other".
  function pick(msg, args) {
    if (typeof msg === 'string') return msg;
    if (!msg || typeof msg !== 'object') return null;
    const n = Number(numeric(args && args[msg.$plural]));
    const cat = Number.isFinite(n) ? new Intl.PluralRules(locale).select(n) : 'other';
    return typeof msg[cat] === 'string' ? msg[cat] : msg.other;
  }

  // numeric unwraps a {num} argument to its number.
  function numeric(v) {
    return v && typeof v === 'object' && 'num' in v ? v.num : v;
  }

  // tokens splits a pattern into text, {var}, {#slot} and {/slot} parts.
  function tokens(pattern) {
    const out = [];
    let last = 0;
    pattern.replace(TOKEN, function (m, kind, name, at) {
      if (at > last) out.push({ kind: 'text', text: pattern.slice(last, at) });
      out.push({ kind: kind === '#' ? 'open' : kind === '/' ? 'close' : 'var', name: name });
      last = at + m.length;
      return m;
    });
    if (last < pattern.length) out.push({ kind: 'text', text: pattern.slice(last) });
    return out;
  }

  // patternFor returns key's pattern for args, or null for an unknown key.
  function patternFor(key, args) {
    return pick(messages[key], args);
  }

  // partText renders a text or {var} part; slot markers render as nothing.
  function partText(p, args) {
    if (p.kind === 'text') return p.text;
    return p.kind === 'var' ? format(args ? args[p.name] : undefined) : '';
  }

  // t renders key as plain text. An unknown key renders as the key itself.
  function t(key, args) {
    const pattern = patternFor(key, args);
    if (pattern == null) return key;
    return tokens(pattern).map(function (p) { return partText(p, args); }).join('');
  }

  // FORMATS render the object argument kinds the server also emits.
  const FORMATS = [
    ['num', function (v) { return fixed(v.num, v.digits); }],
    ['bytes', function (v) { return bytes(v.bytes); }],
    ['seconds', function (v) { return duration(v.seconds, v.style); }],
    ['date', function (v) { return date(v.date, v.style); }],
    ['rel', function (v) { return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(v.rel, v.unit); }],
    ['compact', function (v) { return new Intl.NumberFormat(locale, { notation: 'compact', compactDisplay: 'long', maximumFractionDigits: 0 }).format(v.compact); }],
    ['msg', function (v) { return t(v.msg, v.args); }],
  ];

  // format renders one argument value in the current locale.
  function format(v) {
    if (typeof v === 'number') return new Intl.NumberFormat(locale).format(v);
    if (typeof v === 'string') return v;
    if (!v || typeof v !== 'object') return '';
    const kind = FORMATS.find(function (f) { return f[0] in v; });
    return kind ? kind[1](v) : '';
  }

  function fixed(n, digits) {
    const d = digits || 0;
    return new Intl.NumberFormat(locale, { minimumFractionDigits: d, maximumFractionDigits: d }).format(n);
  }

  // bytes renders a size in the largest unit below 1024 of the next.
  function bytes(n) {
    if (n < 1024) return t('common.size.b', { n: n });
    let f = n;
    let i = -1;
    do {
      f /= 1024;
      i++;
    } while (f >= 1024 && i < BYTE_UNITS.length - 1);
    return t('common.size.' + BYTE_UNITS[i], { n: { num: f, digits: 1 } });
  }

  // duration renders seconds in their largest whole unit.
  function duration(sec, style) {
    const s = style === 'short' ? 'short' : 'long';
    const unit = DURATION_UNITS.find(function (u) { return sec > 0 && sec % u[1] === 0; });
    if (!unit) return t('common.duration.' + s + '.seconds', { n: 0 });
    return t('common.duration.' + s + '.' + unit[0], { n: sec / unit[1] });
  }

  function date(iso, style) {
    const when = new Date(iso);
    if (Number.isNaN(when.getTime())) return '';
    return when.toLocaleString(locale, DATE_STYLES[style] || DATE_STYLES.datetime);
  }

  function readArgs(node) {
    const raw = node.getAttribute('data-i18n-args');
    try {
      return raw ? JSON.parse(raw) : undefined;
    } catch {
      return undefined;
    }
  }

  // renderNode rewrites a data-i18n node's text. Rich-text slots keep their
  // elements (and attributes); only the words in and around them change. A
  // slot left empty in the message keeps its content, such as a <time> a
  // script fills in. An unknown key shows as itself, so it is easy to spot.
  function renderNode(node) {
    const key = node.getAttribute('data-i18n');
    const args = readArgs(node);
    const pattern = patternFor(key, args);
    const parts = pattern == null ? [] : tokens(pattern);
    if (!parts.some(function (p) { return p.kind === 'open'; })) {
      node.textContent = t(key, args);
      return;
    }
    node.replaceChildren.apply(node, richNodes(node, parts, args));
  }

  // richNodes builds a rich message's child nodes, reusing node's slots.
  function richNodes(node, parts, args) {
    const slots = {};
    node.querySelectorAll('[data-i18n-slot]').forEach(function (el) { slots[el.getAttribute('data-i18n-slot')] = el; });
    const out = [];
    let open = null;
    parts.forEach(function (p) {
      if (p.kind === 'open') {
        open = { el: slots[p.name] || null, text: '' };
      } else if (p.kind === 'close') {
        out.push(fillSlot(open.el, open.text));
        open = null;
      } else if (open) {
        open.text += partText(p, args);
      } else {
        out.push(document.createTextNode(partText(p, args)));
      }
    });
    return out;
  }

  // fillSlot sets a slot's words, or returns the words alone when the slot
  // element is missing.
  function fillSlot(el, text) {
    if (!el) return document.createTextNode(text);
    if (text !== '') el.textContent = text;
    return el;
  }

  // renderAttrs rewrites attributes listed as "attr:key;attr:key".
  function renderAttrs(node) {
    const args = readArgs(node);
    node.getAttribute('data-i18n-attr').split(';').forEach(function (pair) {
      const i = pair.indexOf(':');
      if (i > 0) node.setAttribute(pair.slice(0, i), t(pair.slice(i + 1), args));
    });
  }

  function renderValue(node) {
    try {
      node.textContent = format(JSON.parse(node.getAttribute('data-i18n-value')));
    } catch {
      // leave malformed values as rendered
    }
  }

  function renderHidden(node) {
    node.hidden = node.getAttribute('data-i18n-hidden-in').split(' ').indexOf(locale) >= 0;
  }

  // apply re-renders every translated node under root (default: the page).
  function apply(root) {
    const scope = root || document;
    scope.querySelectorAll('[data-i18n]').forEach(renderNode);
    scope.querySelectorAll('[data-i18n-attr]').forEach(renderAttrs);
    scope.querySelectorAll('[data-i18n-value]').forEach(renderValue);
    scope.querySelectorAll('[data-i18n-hidden-in]').forEach(renderHidden);
  }

  function setArgs(node, args) {
    if (args) node.setAttribute('data-i18n-args', JSON.stringify(args));
    else node.removeAttribute('data-i18n-args');
  }

  // set shows key on node and remembers it, so a language change re-renders it.
  function set(node, key, args) {
    if (!node) return;
    node.setAttribute('data-i18n', key);
    setArgs(node, args);
    renderNode(node);
  }

  // setAttr shows key in node's attribute attr and remembers it.
  function setAttr(node, attr, key, args) {
    if (!node) return;
    const pairs = (node.getAttribute('data-i18n-attr') || '').split(';').filter(function (p) {
      return p && p.indexOf(attr + ':') !== 0;
    });
    pairs.push(attr + ':' + key);
    node.setAttribute('data-i18n-attr', pairs.join(';'));
    if (args) setArgs(node, args);
    node.setAttribute(attr, t(key, args));
  }

  // value shows a formatted value (number, size, duration, date) on node and
  // remembers it, so a language change re-formats it.
  function value(node, v) {
    if (!node) return;
    node.setAttribute('data-i18n-value', JSON.stringify(v));
    node.textContent = format(v);
  }

  // plain shows text that is not translated (such as a name the visitor
  // typed) and forgets any message key on node.
  function plain(node, text) {
    if (!node) return;
    node.removeAttribute('data-i18n');
    node.removeAttribute('data-i18n-args');
    node.removeAttribute('data-i18n-value');
    node.textContent = text;
  }

  // snapshot remembers what node shows (its key and args, or its plain text)
  // so restore can put it back later, in whatever language is current then.
  function snapshot(node) {
    if (!node) return null;
    return { key: node.getAttribute('data-i18n'), args: readArgs(node), text: node.textContent };
  }

  function restore(node, snap) {
    if (!node || !snap) return;
    if (snap.key) set(node, snap.key, snap.args);
    else plain(node, snap.text);
  }

  // clear empties node and forgets any message key.
  function clear(node) {
    plain(node, '');
  }

  // setTitle sets the document title from key.
  function setTitle(key, args) {
    const el = document.querySelector('title');
    if (el) set(el, key, args);
    document.title = t(key, args);
  }

  function writeCookie(tag) {
    const secure = window.location && window.location.protocol === 'https:' ? '; Secure' : '';
    document.cookie = COOKIE + '=' + encodeURIComponent(tag) + '; Path=/; Max-Age=31536000; SameSite=Lax' + secure;
  }

  // loadCatalog fetches tag's full catalog once.
  async function loadCatalog(tag) {
    if (catalogs[tag]) return catalogs[tag];
    const url = data.catalogs && data.catalogs[tag];
    if (!url) throw new Error('unknown locale');
    const res = await fetch(url, { credentials: 'same-origin' });
    if (!res.ok) throw new Error('catalog ' + res.status);
    catalogs[tag] = await res.json();
    return catalogs[tag];
  }

  // setLocale switches the page to tag in place and saves the choice.
  // Resolves to true when the page changed, false when tag is unknown or its
  // catalog could not be fetched (the page stays as it was).
  async function setLocale(tag) {
    if (tag === locale) return true;
    let catalog;
    try {
      catalog = await loadCatalog(tag);
    } catch {
      return false;
    }
    messages = catalog;
    locale = tag;
    const html = document.documentElement;
    html.lang = tag;
    html.dir = dirFor(tag);
    writeCookie(tag);
    apply(document);
    listeners.slice().forEach(function (fn) { fn(tag); });
    return true;
  }

  function dirFor(tag) {
    const found = (data.locales || []).find(function (l) { return l.tag === tag; });
    return found && found.dir === 'rtl' ? 'rtl' : 'ltr';
  }

  // onChange calls fn with the new tag after each language change.
  function onChange(fn) {
    listeners.push(fn);
  }

  window.goneI18n = Object.freeze({
    get locale() { return locale; },
    locales: function () { return (data.locales || []).slice(); },
    t: t,
    format: format,
    set: set,
    setAttr: setAttr,
    value: value,
    plain: plain,
    clear: clear,
    snapshot: snapshot,
    restore: restore,
    setTitle: setTitle,
    apply: apply,
    setLocale: setLocale,
    onChange: onChange,
  });
})();
