'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');

reset();
load('fileMeta');
const meta = window.goneFileMeta;

test('module loads once', () => {
  load('fileMeta');
  assert.equal(window.goneFileMeta, meta);
});

test('sanitizeFileName', () => {
  const cases = [
    ['report.pdf', 'report.pdf'],
    ['dir/sub/name.txt', 'name.txt'],
    ['C:\\Users\\me\\a.txt', 'a.txt'],
    ['trailing/', 'trailing'],
    ['', 'file'],
    [null, 'file'],
    ['..', 'file'],
    ['.', 'file'],
    ['   ', 'file'],
    ['a\u0000b\u202ec\u200bd\ufeff', 'abcd'],
    ['\u0007\u009f', 'file'],
    ['emoji \ud83d\ude00.png', 'emoji \ud83d\ude00.png']
  ];
  for (const [input, want] of cases) {
    assert.equal(meta.sanitizeFileName(input), want, String(input));
  }
  assert.equal(Array.from(meta.sanitizeFileName('\ud83d\ude00'.repeat(300))).length, 255);
});

test('safeType', () => {
  const cases = [
    ['image/png', 'image/png'],
    ['TEXT/PLAIN; charset=utf-8', 'text/plain'],
    ['text/html', 'application/octet-stream'],
    ['image/svg+xml', 'application/octet-stream'],
    ['', 'application/octet-stream'],
    [undefined, 'application/octet-stream']
  ];
  for (const [input, want] of cases) assert.equal(meta.safeType(input), want, String(input));
});

test('formatBytes', () => {
  const cases = [
    [0, '0 B'], [1023, '1023 B'], [1024, '1.0 KB'], [1536, '1.5 KB'], [10 * 1024, '10 KB'],
    [5 * 1024 * 1024, '5.0 MB'], [3 * 1024 ** 3, '3.0 GB'], [5000 * 1024 ** 3, '5000 GB'],
    [-1, '0 B'], [NaN, '0 B'], [Infinity, '0 B']
  ];
  for (const [input, want] of cases) assert.equal(meta.formatBytes(input), want, String(input));
});
