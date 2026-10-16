'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');

function v3() {
  reset();
  load('crypto', 'cryptoV3');
  return window.goneCryptoV3;
}

test('generateKeyPair keeps the private key non-extractable', async () => {
  const m = v3();
  const kp = await m.generateKeyPair();
  assert.equal(kp.publicKey.length, 65);
  assert.equal(kp.publicKey[0], 4);
  assert.equal(kp.privateKey.extractable, false);
  await assert.rejects(crypto.subtle.exportKey('jwk', kp.privateKey));
});

test('encryptV3 round-trips with a fresh ephemeral key each time', async () => {
  const m = v3();
  const kp = await m.generateKeyPair();
  const a = await m.encryptV3('hello', kp.publicKey);
  const b = await m.encryptV3('hello', kp.publicKey);
  assert.notDeepEqual(a.ciphertext.subarray(0, 65), b.ciphertext.subarray(0, 65));
  assert.equal(a.nonce.length, 12);
  const pt = await m.decryptV3(a.ciphertext, a.nonce, kp.privateKey, kp.publicKey);
  assert.equal(new TextDecoder().decode(pt), 'hello');
});

test('encryptV3 rejects a public key that is not on the curve', async () => {
  const m = v3();
  const bad = new Uint8Array(65);
  bad[0] = 4;
  await assert.rejects(m.encryptV3('x', bad), (e) => e.code === 'invalid_key');
});

test('decryptV3 reports one generic error', async () => {
  const m = v3();
  const kp = await m.generateKeyPair();
  const other = await m.generateKeyPair();
  const enc = await m.encryptV3('x', kp.publicKey);
  const cases = [
    [enc.ciphertext.subarray(0, 80), enc.nonce, kp.privateKey],
    [enc.ciphertext, enc.nonce.subarray(0, 11), kp.privateKey],
    [enc.ciphertext, null, kp.privateKey],
    [null, enc.nonce, kp.privateKey],
    [enc.ciphertext, enc.nonce, other.privateKey]
  ];
  for (const [blob, nonce, key] of cases) {
    await assert.rejects(m.decryptV3(blob, nonce, key, kp.publicKey), (e) => e.code === 'decrypt');
  }
});

test('parseReplyFragment rejects non-strings and bad base64', () => {
  const m = v3();
  assert.throws(() => m.parseReplyFragment(null), (e) => e.code === 'invalid_fragment');
  assert.throws(() => m.parseReplyFragment('v3:A.' + 'A'.repeat(43)), (e) => e.code === 'invalid_fragment');
});

test('encryptV3 keeps the ephemeral private key non-extractable', async (t) => {
  const m = v3();
  const kp = await m.generateKeyPair();
  const made = [];
  const generate = crypto.subtle.generateKey.bind(crypto.subtle);
  t.mock.method(crypto.subtle, 'generateKey', async (...args) => {
    const pair = await generate(...args);
    made.push(pair);
    return pair;
  });
  const enc = await m.encryptV3('x', kp.publicKey);
  assert.equal(made.length, 1);
  assert.equal(made[0].privateKey.extractable, false);
  await assert.rejects(crypto.subtle.exportKey('pkcs8', made[0].privateKey));
  const pubE = new Uint8Array(await crypto.subtle.exportKey('raw', made[0].publicKey));
  assert.deepEqual(enc.ciphertext.subarray(0, 65), pubE);
});

test('encryptV3 accepts only an uncompressed requester point', async () => {
  const m = v3();
  const kp = await m.generateKeyPair();
  const pub = kp.publicKey;
  const compressed = new Uint8Array(33);
  compressed[0] = 2 | (pub[64] & 1);
  compressed.set(pub.subarray(1, 33), 1);
  const hybrid = pub.slice();
  hybrid[0] = 6 | (pub[64] & 1);
  for (const bad of [compressed, hybrid, Array.from(pub), null]) {
    await assert.rejects(m.encryptV3('x', bad), (e) => e.code === 'invalid_key');
  }
});

test('decryptV3 rejects an ephemeral point that is not uncompressed', async () => {
  const m = v3();
  const kp = await m.generateKeyPair();
  const enc = await m.encryptV3('x', kp.publicKey);
  const blob = enc.ciphertext.slice();
  blob[0] = 6 | (blob[64] & 1);
  await assert.rejects(m.decryptV3(blob, enc.nonce, kp.privateKey, kp.publicKey), (e) => e.code === 'decrypt');
});

test('parseReplyFragment requires a strict base64url fill token', () => {
  const m = v3();
  const key = new Uint8Array(65);
  key[0] = 4;
  const ok = m.replyFragment(key, 'A'.repeat(43));
  assert.equal(m.parseReplyFragment(ok).fill, 'A'.repeat(43));
  assert.throws(() => m.parseReplyFragment(ok.slice(0, -1) + 'B'), (e) => e.code === 'invalid_fragment');
});

test('cryptoV3 waits for its dependencies and loads once', () => {
  reset();
  load('cryptoV3');
  assert.equal(window.goneCryptoV3, undefined);
  load('crypto', 'cryptoV3');
  const first = window.goneCryptoV3;
  load('cryptoV3');
  assert.equal(window.goneCryptoV3, first);
});
