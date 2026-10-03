'use strict';

// Passphrase helpers for the optional protocol v2 passphrase. generate()
// draws CamelCase words from the EFF short wordlist (window.goneWordlist)
// with unbiased rejection sampling; strength() is a local, coarse estimate
// for the sender's hint and never leaves the page. Exposed as
// window.gonePassgen.
(function passgen() {
  const WORDS = 5;
  const MIN_CHARS = 8;
  const FAIR_BITS = 45;
  const STRONG_BITS = 70;
  const RANGE = 0x10000;
  if (window.gonePassgen || !window.goneWordlist) return;
  const list = window.goneWordlist;
  const limit = RANGE - (RANGE % list.length);

  // pickIndex returns a uniform index into list. Values at or above the
  // largest multiple of list.length are redrawn so no word is favoured.
  function pickIndex() {
    const buf = new Uint16Array(1);
    for (;;) {
      crypto.getRandomValues(buf);
      if (buf[0] < limit) return buf[0] % list.length;
    }
  }

  function capitalize(w) {
    return w.charAt(0).toUpperCase() + w.slice(1);
  }

  // generate returns WORDS CamelCase words, e.g. "FrostCanalBloomTrickRuby"
  // (about 51.7 bits from a 1,296-word list).
  function generate() {
    let out = '';
    for (let i = 0; i < WORDS; i++) {
      out += capitalize(list[pickIndex()]);
    }
    return out;
  }

  // poolSize estimates the alphabet a passphrase draws from, by character
  // class: lowercase, uppercase, digits, ASCII symbols, and anything else.
  function poolSize(chars) {
    const classes = [[/[a-z]/, 26], [/[A-Z]/, 26], [/[0-9]/, 10], [/[ -/:-@[-`{-~]/, 33], [/[^\x20-\x7e]/, 100]];
    let pool = 0;
    classes.forEach(([re, n]) => {
      if (chars.some((c) => re.test(c))) pool += n;
    });
    return pool;
  }

  // strength rates a passphrase as "empty", "short" (under MIN_CHARS code
  // points), "weak", "fair", or "strong". Bits are length times log2 of the
  // class pool, counting repeated neighbours once, so "aaaaaaaa" rates weak.
  function strength(p) {
    const chars = Array.from(typeof p === 'string' ? p : '');
    if (chars.length === 0) return 'empty';
    if (chars.length < MIN_CHARS) return 'short';
    const distinct = chars.filter((c, i) => i === 0 || c !== chars[i - 1]).length;
    const bits = distinct * Math.log2(poolSize(chars));
    if (bits >= STRONG_BITS) return 'strong';
    return bits >= FAIR_BITS ? 'fair' : 'weak';
  }

  window.gonePassgen = Object.freeze({
    minChars: MIN_CHARS,
    generate: generate,
    strength: strength
  });
})();
