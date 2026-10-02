'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');

const t0 = test.mock.method(console, 'log', () => {});
reset();
load('crypto');
t0.mock.restore();
const gc = window.goneCrypto;

test('module loads once and is frozen', () => {
  load('crypto');
  assert.equal(window.goneCrypto, gc);
  assert.ok(Object.isFrozen(gc));
  assert.equal(gc.version, 1);
});

test('b64url round trips without padding or unsafe chars', () => {
  for (let n = 0; n < 40; n++) {
    const bytes = crypto.getRandomValues(new Uint8Array(n));
    const enc = gc.b64urlEncode(bytes);
    assert.doesNotMatch(enc, /[+/=]/);
    assert.deepEqual(gc.b64urlDecode(enc), bytes);
  }
  assert.equal(gc.b64urlEncode(new Uint8Array([0xfb, 0xff])), '-_8');
});

test('key generation and import/export', () => {
  const key = gc.generateKey();
  assert.equal(key.length, 32);
  assert.deepEqual(gc.importKeyB64(gc.exportKeyB64(key)), key);
  assert.throws(() => gc.importKeyB64('AAAA'), /invalid key/);
});

test('encrypt/decrypt round trip for strings and bytes', async () => {
  const key = gc.generateKey();
  const s = await gc.encrypt('héllo', key);
  assert.equal(s.nonce.length, 12);
  assert.equal(new TextDecoder().decode(await gc.decrypt(s.ciphertext, s.nonce, key)), 'héllo');
  const bytes = new Uint8Array([1, 2, 3]);
  const b = await gc.encrypt(bytes, key);
  assert.equal(b.ciphertext.length, 3 + 16);
  assert.deepEqual(await gc.decrypt(b.ciphertext, b.nonce, key), bytes);
  const again = await gc.encrypt(bytes, key);
  assert.notDeepEqual(again.nonce, b.nonce);
});

test('encrypt rejects bad input', async () => {
  const key = gc.generateKey();
  await assert.rejects(gc.encrypt('x', new Uint8Array(16)), /invalid key length/);
  await assert.rejects(gc.encrypt('x', 'not-bytes'), /invalid key length/);
  await assert.rejects(gc.encrypt(42, key), /data must be string or Uint8Array/);
});

test('decrypt rejects tampering, wrong keys and bad nonces', async () => {
  const key = gc.generateKey();
  const s = await gc.encrypt('secret', key);
  await assert.rejects(gc.decrypt(s.ciphertext, s.nonce, gc.generateKey()));
  const tampered = s.ciphertext.slice();
  tampered[0] ^= 1;
  await assert.rejects(gc.decrypt(tampered, s.nonce, key));
  await assert.rejects(gc.decrypt(s.ciphertext, new Uint8Array(8), key), /invalid nonce length/);
  await assert.rejects(gc.decrypt(s.ciphertext, s.nonce, new Uint8Array(31)), /invalid key length/);
});
