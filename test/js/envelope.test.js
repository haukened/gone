'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');

const MAGIC = [0x47, 0x4f, 0x4e, 0x45, 0x32, 0, 0, 0];

reset();
load('fileMeta', 'envelope');
const env = window.goneEnvelope;
const enc = (s) => new TextEncoder().encode(s);

// raw builds an envelope from an arbitrary header object and body bytes.
function raw(header, body, lenOverride) {
  let hb;
  if (header instanceof Uint8Array) hb = header;
  else hb = typeof header === 'string' ? enc(header) : enc(JSON.stringify(header));
  const out = new Uint8Array(12 + hb.length + body.length);
  out.set(MAGIC, 0);
  new DataView(out.buffer).setUint32(8, lenOverride === undefined ? hb.length : lenOverride);
  out.set(hb, 12);
  out.set(body, 12 + hb.length);
  return out;
}

test('requires fileMeta and loads once', () => {
  load('envelope');
  assert.equal(window.goneEnvelope, env);
  const saved = window.goneFileMeta;
  delete window.goneEnvelope;
  delete window.goneFileMeta;
  load('envelope');
  assert.equal(window.goneEnvelope, undefined);
  window.goneFileMeta = saved;
  window.goneEnvelope = env;
});

test('re-exports file metadata helpers', () => {
  assert.equal(env.MAX_FILES, 10);
  assert.equal(env.sanitizeFileName, window.goneFileMeta.sanitizeFileName);
  assert.equal(env.safeType, window.goneFileMeta.safeType);
  assert.equal(env.formatBytes, window.goneFileMeta.formatBytes);
});

test('text-only secrets are raw UTF-8 (legacy compatible)', () => {
  for (const files of [undefined, []]) {
    const out = env.encode('héllo', files);
    assert.deepEqual(out, enc('héllo'));
    assert.deepEqual(env.decode(out), { message: 'héllo', files: [] });
  }
  assert.deepEqual(env.encode(undefined), new Uint8Array(0));
  assert.deepEqual(env.decode(new Uint8Array([0x47, 0x4f])), { message: 'GO', files: [] });
});

test('round trips message and files with sanitized metadata', () => {
  const files = [
    { name: '../a.txt', type: 'text/plain', bytes: enc('AAA') },
    { name: 'b.html', type: 'text/html', bytes: new Uint8Array([1, 2]) },
    { name: 'empty', type: '', bytes: new Uint8Array(0) }
  ];
  const out = env.encode('msg', files);
  assert.deepEqual(Array.from(out.subarray(0, 8)), MAGIC);
  const header = JSON.parse(new TextDecoder().decode(out.subarray(12, 12 + new DataView(out.buffer).getUint32(8))));
  assert.deepEqual(header, {
    v: 2, msg: 3, files: [
      { name: 'a.txt', type: 'text/plain', size: 3 },
      { name: 'b.html', type: 'application/octet-stream', size: 2 },
      { name: 'empty', type: 'application/octet-stream', size: 0 }
    ]
  });
  const dec = env.decode(out);
  assert.equal(dec.message, 'msg');
  assert.deepEqual(dec.files.map((f) => [f.name, f.type, f.size, Array.from(f.bytes)]), [
    ['a.txt', 'text/plain', 3, [65, 65, 65]],
    ['b.html', 'application/octet-stream', 2, [1, 2]],
    ['empty', 'application/octet-stream', 0, []]
  ]);
  assert.deepEqual(env.decode(env.encode('', [files[0]])).message, '');
});

test('encryptedSize matches the encoded length plus the GCM tag', () => {
  const files = [{ name: 'x.bin', type: 'image/png', bytes: new Uint8Array(1000) }];
  const metas = files.map((f) => ({ name: f.name, type: f.type, size: f.bytes.length }));
  assert.equal(env.encryptedSize('hi', metas), env.encode('hi', files).length + 16);
  assert.equal(env.encryptedSize('hé', []), 3 + 16);
  assert.equal(env.encryptedSize(undefined, []), 16);
});

test('encode limits', () => {
  const file = { name: 'f', type: '', bytes: new Uint8Array(1) };
  assert.throws(() => env.encode('', Array(11).fill(file)), /too many files/);
  const long = { name: 'n'.repeat(255), type: '', bytes: new Uint8Array(0) };
  // Ten 255-char names fit; the header limit is reached via the JSON size.
  assert.doesNotThrow(() => env.encode('', Array(10).fill(long)));
  const wide = { name: '\u00e9'.repeat(255), type: '', bytes: new Uint8Array(0) };
  assert.doesNotThrow(() => env.encode('', Array(10).fill(wide)));
});

test('encode rejects headers over 16 KiB', () => {
  // Real sanitized names cannot exceed the limit; stub fileMeta to prove the guard.
  const saved = { meta: window.goneFileMeta, env: window.goneEnvelope };
  delete window.goneEnvelope;
  window.goneFileMeta = { sanitizeFileName: (n) => n, safeType: (t) => t, formatBytes: String };
  load('envelope');
  const big = { name: 'n'.repeat(17 * 1024), type: '', bytes: new Uint8Array(0) };
  assert.throws(() => window.goneEnvelope.encode('', [big]), /header too large/);
  window.goneFileMeta = saved.meta;
  window.goneEnvelope = saved.env;
});

test('decode rejects malformed envelopes', () => {
  const ok = { v: 2, msg: 1, files: [{ name: 'a', type: '', size: 1 }] };
  const body = new Uint8Array([9, 9]);
  const cases = [
    ['truncated', new Uint8Array(MAGIC.concat([0, 0])), /truncated envelope/],
    ['length past end', raw(ok, body, 9999), /invalid header length/],
    ['length over max', raw(ok, body, 16 * 1024 + 1), /invalid header length/],
    ['bad json', raw('{nope', body), SyntaxError],
    ['bad utf8', raw(new Uint8Array([0xff]), body), TypeError],
    ['null header', raw(null, body), /invalid envelope header/],
    ['wrong version', raw({ v: 1, msg: 1, files: [] }, body), /invalid envelope header/],
    ['bad msg', raw({ v: 2, msg: -1, files: [] }, body), /invalid envelope header/],
    ['files not array', raw({ v: 2, msg: 1, files: {} }, body), /invalid envelope header/],
    ['too many', raw({ v: 2, msg: 0, files: Array(11).fill({ size: 0 }) }, new Uint8Array(0)), /too many files/],
    ['null file', raw({ v: 2, msg: 0, files: [null] }, body), /invalid file entry/],
    ['bad size', raw({ v: 2, msg: 0, files: [{ size: 1.5 }] }, body), /invalid file entry/],
    ['mismatch', raw(ok, new Uint8Array(3)), /envelope size mismatch/]
  ];
  for (const [name, bytes, want] of cases) {
    assert.throws(() => env.decode(bytes), want, name);
  }
  assert.doesNotThrow(() => env.decode(raw(ok, body)));
});

test('raw magic check handles a missing buffer', () => {
  assert.throws(() => env.decode(null));
});
