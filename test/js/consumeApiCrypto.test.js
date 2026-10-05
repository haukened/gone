'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { fakeResponse, installFetch } = require('./fakes');
const { URL_FOR_ID, setup } = require('./consumeApiHarness');

test('decrypt verifies version, nonce and key, then zeroes buffers', async (t) => {
  const { api } = setup(t);
  const gc = window.goneCrypto;
  const key = gc.generateKey();
  const frag = () => ({ version: 1, key: key.slice() });
  const enc = await gc.encrypt('hello', key);
  const headers = (v, nonce) => fakeResponse({ headers: Object.assign({ 'X-Gone-Version': v }, nonce === undefined ? {} : { 'X-Gone-Nonce': nonce }) });
  const nonce = gc.b64urlEncode(enc.nonce);

  const ct = enc.ciphertext.slice();
  const f = frag();
  const pt = await api.decrypt(headers('1', nonce), ct, f);
  assert.equal(new TextDecoder().decode(pt), 'hello');
  assert.ok(ct.every((b) => b === 0));
  assert.ok(f.key.every((b) => b === 0));

  const unsupported = (e) => e.message === 'Unsupported secret version' && !e.retryable;
  for (const v of ['2', '01', '+1', '1.0']) {
    await assert.rejects(api.decrypt(headers(v, nonce), enc.ciphertext.slice(), frag()), unsupported, v);
  }
  await assert.rejects(api.decrypt(fakeResponse({}), enc.ciphertext.slice(), frag()), unsupported);
  const verify = (e) => /Couldn.t verify/.test(e.message) && e.retryable === false;
  for (const n of [undefined, '', nonce + '=', nonce.slice(0, -1) + '_', 'AAAA', '!'.repeat(16)]) {
    const g = frag();
    await assert.rejects(api.decrypt(headers('1', n), enc.ciphertext.slice(), g), verify, String(n));
    assert.ok(g.key.every((b) => b === 0));
  }
  await assert.rejects(api.decrypt(headers('1', nonce), enc.ciphertext.slice(), { version: 1, key: gc.generateKey() }), verify);
});

test('decryptV2 retries a wrong passphrase in place and maps other failures', async (t) => {
  const { api } = setup(t);
  const gc = window.goneCrypto;
  const key = gc.generateKey();
  const enc = await gc.encryptV2('hello', key, 'Right horse');
  const nonce = gc.b64urlEncode(enc.nonce);
  const resp = (v, n) => fakeResponse({ headers: { 'X-Gone-Version': v, 'X-Gone-Nonce': n } });
  const zero = (b) => b.every((x) => x === 0);
  const intact = (b) => !zero(b);
  const retry = (e) => e.passphrase === true && e.retryable === true && /passphrase didn.t work/.test(e.message);
  const final = (re) => (e) => re.test(e.message) && e.retryable === false && !e.passphrase;

  // A wrong (or unencodable) passphrase keeps both buffers for a local retry.
  const ct = enc.ciphertext.slice();
  const f = { version: 2, key: key.slice() };
  await assert.rejects(api.decryptV2(resp('2', nonce), ct, f, 'wrong horse'), retry);
  await assert.rejects(api.decryptV2(resp('2', nonce), ct, f, ''), retry);
  assert.ok(intact(ct) && intact(f.key));
  const pt = await api.decryptV2(resp('2', nonce), ct, f, 'Right horse');
  assert.equal(new TextDecoder().decode(pt), 'hello');
  assert.ok(zero(ct) && zero(f.key));

  // Everything else is final and zeroes the buffers.
  const damaged = enc.ciphertext.slice();
  damaged[0] = 9;
  const boom = fakeResponse({});
  boom.headers.get = () => { throw new Error('boom'); };
  const cases = [
    ['v1 header', resp('1', nonce), enc.ciphertext.slice(), final(/^Unsupported secret version$/)],
    ['no nonce', fakeResponse({ headers: { 'X-Gone-Version': '2' } }), enc.ciphertext.slice(), final(/Couldn.t verify/)],
    ['bad nonce', resp('2', '!!'), enc.ciphertext.slice(), final(/Couldn.t verify/)],
    ['short nonce', resp('2', 'AAAA'), enc.ciphertext.slice(), final(/Couldn.t verify/)],
    ['bad header', resp('2', nonce), damaged, final(/contents are damaged/)],
    ['unexpected', boom, enc.ciphertext.slice(), final(/Couldn.t verify/)]
  ];
  for (const [name, r, body, check] of cases) {
    const g = { version: 2, key: key.slice() };
    await assert.rejects(api.decryptV2(r, body, g, 'Right horse'), check, name);
    assert.ok(zero(body) && zero(g.key), name);
  }
});

test('acknowledge sends a keepalive DELETE with the claim', async (t) => {
  const { api, delays } = setup(t);
  const calls = installFetch([() => fakeResponse({ status: 204 })]);
  assert.equal(await api.acknowledge({ token: 'tok' }), true);
  assert.equal(calls[0].url, URL_FOR_ID);
  assert.equal(calls[0].init.method, 'DELETE');
  assert.equal(calls[0].init.keepalive, true);
  assert.deepEqual(calls[0].init.headers, { 'X-Gone-Claim': 'tok' });
  assert.deepEqual(delays, []);
});

test('acknowledge retries non-204 and network errors, then reports failure', async (t) => {
  const { api, delays } = setup(t);
  const calls = installFetch([
    () => fakeResponse({ status: 500 }),
    () => { throw new Error('offline'); },
    () => fakeResponse({ status: 409 })
  ]);
  assert.equal(await api.acknowledge({ token: 'tok' }), false);
  assert.equal(calls.length, 3);
  assert.deepEqual(delays, [500, 1000, 1500]);
});

test('acknowledge treats 404 as already deleted, without retrying', async (t) => {
  const { api, delays } = setup(t);
  const calls = installFetch([() => fakeResponse({ status: 404 })]);
  assert.equal(await api.acknowledge({ token: 'tok' }), true);
  assert.equal(calls.length, 1);
  assert.deepEqual(delays, []);
});

test('acknowledge succeeds on a later attempt', async (t) => {
  const { api } = setup(t);
  installFetch([() => fakeResponse({ status: 500 }), () => fakeResponse({ status: 204 })]);
  assert.equal(await api.acknowledge({ token: 'tok' }), true);
});
