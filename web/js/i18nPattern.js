'use strict';

// Message patterns for goneI18n: plural variant choice and tokenizing a
// pattern into text, {var}, {#slot} and {/slot} parts. Loaded before i18n.js.
// Exposed as window.goneI18nPattern.
(function i18nPatternModule() {
  if (window.goneI18nPattern) return;

  // numeric unwraps a {num} argument to its number.
  function numeric(v) {
    return v && typeof v === 'object' && 'num' in v ? v.num : v;
  }

  // tokens splits a pattern into text, {var}, {#slot} and {/slot} parts.
  function tokens(pattern) {
    const TOKEN = /\{([#/]?)([A-Za-z_][A-Za-z0-9_]*)\}/g;
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

  // create returns pick for ctx, whose locale() is the current language.
  function create(ctx) {
    // pick returns the pattern for a message: the plural variant for its input
    // argument's CLDR category, falling back to "other".
    function pick(msg, args) {
      if (typeof msg === 'string') return msg;
      if (!msg || typeof msg !== 'object') return null;
      const n = Number(numeric(args && args[msg.$plural]));
      const cat = Number.isFinite(n) ? new Intl.PluralRules(ctx.locale()).select(n) : 'other';
      return typeof msg[cat] === 'string' ? msg[cat] : msg.other;
    }

    return { pick: pick };
  }

  window.goneI18nPattern = Object.freeze({ tokens: tokens, numeric: numeric, create: create });
})();
