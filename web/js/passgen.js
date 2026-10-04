'use strict';

// Passphrase helpers for the optional protocol v2 passphrase. generate()
// draws CamelCase words from the EFF short wordlist (window.goneWordlist)
// with unbiased rejection sampling; bits(), strength() and crackSeconds()
// are local, coarse estimates for the sender's hint and never leave the
// page. Exposed as window.gonePassgen.
(function passgen() {
  const WORDS = 5;
  const MIN_CHARS = 8;
  const FAIR_BITS = 40;
  const STRONG_BITS = 50;
  // DICT_WORD_BITS is one word from a large diceware list (7,776 words), the
  // cost assumed for each word of a typed multi-word passphrase.
  const DICT_WORD_BITS = Math.log2(7776);
  const RANGE = 0x10000;
  // GUESSES_PER_SECOND models an offline attacker who already holds the link:
  // a rack of GPUs against PBKDF2-SHA256 at 600,000 rounds.
  const GUESSES_PER_SECOND = 1000000;
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

  // wordCount returns how many alphabetic words a passphrase is made of,
  // split on spaces, punctuation and CamelCase, or 0 when any piece is not
  // purely alphabetic.
  function wordCount(chars) {
    const parts = chars.join('').split(/[\s\-_.,]+/).filter(Boolean)
      .flatMap((w) => w.split(/(?=[A-Z])/));
    return parts.every((w) => /^[A-Za-z]+$/.test(w)) ? parts.length : 0;
  }

  // bits estimates a passphrase's entropy. The base estimate is length times
  // log2 of the character-class pool, counting repeated neighbours once, so
  // "aaaaaaaa" scores low. A phrase of two or more dictionary-style words is
  // capped at DICT_WORD_BITS per word, since attackers guess words, not
  // letters. Returns 0 for an empty or non-string value.
  function bits(p) {
    const chars = Array.from(typeof p === 'string' ? p : '');
    if (chars.length === 0) return 0;
    const distinct = chars.filter((c, i) => i === 0 || c !== chars[i - 1]).length;
    const byChars = distinct * Math.log2(poolSize(chars));
    const words = wordCount(chars);
    return words >= 2 ? Math.min(byChars, words * DICT_WORD_BITS) : byChars;
  }

  // strength rates a passphrase as "empty", "short" (under MIN_CHARS code
  // points), "weak", "fair", or "strong". knownBits, when given, replaces the
  // estimate, e.g. for a generated passphrase whose entropy is exact.
  function strength(p, knownBits) {
    const chars = Array.from(typeof p === 'string' ? p : '');
    if (chars.length === 0) return 'empty';
    if (chars.length < MIN_CHARS) return 'short';
    const b = typeof knownBits === 'number' ? knownBits : bits(p);
    if (b >= STRONG_BITS) return 'strong';
    return b >= FAIR_BITS ? 'fair' : 'weak';
  }

  // crackSeconds returns the average time, in seconds, to guess a secret of
  // the given entropy (half the space) at GUESSES_PER_SECOND.
  function crackSeconds(entropyBits) {
    return Math.pow(2, Math.max(0, entropyBits) - 1) / GUESSES_PER_SECOND;
  }

  window.gonePassgen = Object.freeze({
    minChars: MIN_CHARS,
    generatedBits: WORDS * Math.log2(list.length),
    guessesPerSecond: GUESSES_PER_SECOND,
    generate: generate,
    bits: bits,
    strength: strength,
    crackSeconds: crackSeconds
  });
})();
