'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');

// boot loads the wordlist and generator into a fresh window.
function boot() {
  reset();
  load('wordlist', 'passgen');
  return window.gonePassgen;
}

test('wordlist is the frozen 1,296-word EFF short list', () => {
  boot();
  const w = window.goneWordlist;
  assert.ok(Object.isFrozen(w));
  assert.equal(w.length, 1296);
  assert.equal(new Set(w).size, 1296);
  assert.equal(w[0], 'acid');
  assert.equal(w[1295], 'zoom');
  w.forEach((x) => assert.match(x, /^[a-z]+(-[a-z]+)?$/));
  load('wordlist');
  assert.equal(window.goneWordlist, w);
});

test('passgen requires the wordlist and loads once', () => {
  reset();
  load('passgen');
  assert.equal(window.gonePassgen, undefined);
  const pg = boot();
  assert.ok(Object.isFrozen(pg));
  assert.equal(pg.minChars, 8);
  load('passgen');
  assert.equal(window.gonePassgen, pg);
});

test('generate CamelCases five words with rejection sampling', (t) => {
  const pg = boot();
  const draws = [65535, 64800, 0, 1, 1295, 1296, 64799];
  const fill = t.mock.method(globalThis.crypto, 'getRandomValues', (arr) => { arr[0] = draws.shift(); return arr; });
  assert.equal(pg.generate(), 'AcidAcornZoomAcidZoom');
  assert.equal(fill.mock.callCount(), 7);
  fill.mock.restore();
  const p = pg.generate();
  assert.match(p, /^([A-Z][a-z]*(-[a-z]+)?){5}$/);
  assert.equal(pg.strength(p), 'strong');
});

test('strength rates by length, character pool, and repeats', () => {
  const pg = boot();
  const cases = [
    ['', 'empty'], [null, 'empty'], ['abc', 'short'], ['\u00e9t\u00e9', 'short'],
    ['aaaaaaaaaaaa', 'weak'], ['password', 'weak'], ['Password', 'weak'],
    ['lowercaseonly', 'fair'], ['Abcdefg1', 'fair'],
    ['correct horse battery staple', 'strong'], ['FrostCanalBloomTrickRuby', 'strong'],
    ['\u00fcberstra\u00dfe!', 'strong']
  ];
  cases.forEach(([p, want]) => assert.equal(pg.strength(p), want, JSON.stringify(p)));
});
