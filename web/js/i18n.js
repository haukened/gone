'use strict';

// Translations. The server renders each page in the visitor's language and
// marks every translated node with its message key (data-i18n, plus
// data-i18n-args, data-i18n-attr, data-i18n-value). This module renders the
// same messages in the browser, so text set by scripts is translated too and
// the language can change in place, without a reload that would lose a
// share link, a revealed secret, or typed text.
//
// Loaded in <head>, after the #gone-i18n JSON block, its parts
// (i18nPattern, i18nFormat, i18nRender, i18nNodes, i18nLocale) and before
// every other script, so modules can look up messages as they start. This
// file holds the current language and messages and joins the parts. Exposed
// as window.goneI18n.
(function i18nModule() {
  const PARTS = ['goneI18nPattern', 'goneI18nFormat', 'goneI18nRender', 'goneI18nNodes', 'goneI18nLocale'];
  if (window.goneI18n || !PARTS.every(function (name) { return window[name]; })) return;

  const data = readData();
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

  // ctx is what the parts share: the live locale and the message renderers.
  const ctx = {
    locale: function () { return locale; },
    t: t,
    partText: partText,
    patternFor: patternFor,
    tokens: window.goneI18nPattern.tokens
  };
  const pattern = window.goneI18nPattern.create(ctx);
  ctx.format = window.goneI18nFormat.create(ctx).format;
  const render = window.goneI18nRender.create(ctx);
  ctx.readArgs = render.readArgs;
  ctx.renderNode = render.renderNode;
  const nodes = window.goneI18nNodes.create(ctx);
  const locales = window.goneI18nLocale.create(data);

  // patternFor returns key's pattern for args, or null for an unknown key.
  function patternFor(key, args) {
    return pattern.pick(messages[key], args);
  }

  // partText renders a text or {var} part; slot markers render as nothing.
  function partText(p, args) {
    if (p.kind === 'text') return p.text;
    return p.kind === 'var' ? ctx.format(args ? args[p.name] : undefined) : '';
  }

  // t renders key as plain text. An unknown key renders as the key itself.
  function t(key, args) {
    const found = patternFor(key, args);
    if (found == null) return key;
    return ctx.tokens(found).map(function (p) { return partText(p, args); }).join('');
  }

  // setLocale switches the page to tag in place and saves the choice.
  // Resolves to true when the page changed, false when tag is unknown or its
  // catalog could not be fetched (the page stays as it was).
  async function setLocale(tag) {
    if (tag === locale) return true;
    let catalog;
    try {
      catalog = await locales.loadCatalog(tag);
    } catch {
      return false;
    }
    messages = catalog;
    locale = tag;
    const html = document.documentElement;
    html.lang = tag;
    html.dir = locales.dirFor(tag);
    locales.writeCookie(tag);
    render.apply(document);
    listeners.slice().forEach(function (fn) { fn(tag); });
    return true;
  }

  // onChange calls fn with the new tag after each language change.
  function onChange(fn) {
    listeners.push(fn);
  }

  // api is the public interface; locale is live.
  function api() {
    return Object.assign({
      get locale() { return locale; },
      locales: function () { return (data.locales || []).slice(); },
      t: t,
      format: ctx.format,
      apply: render.apply,
      setLocale: setLocale,
      onChange: onChange,
    }, nodes);
  }

  window.goneI18n = Object.freeze(api());
})();
