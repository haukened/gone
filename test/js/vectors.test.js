'use strict';

// Shared protocol vectors (test/vectors, docs/protocol.md section 9). The Go
// reference implementation generates them; the browser code must agree.

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { reset, load } = require('./harness');

const VECTOR_DIR = path.join(__dirname, '..', 'vectors');

const readVectors = (name) => JSON.parse(fs.readFileSync(path.join(VECTOR_DIR, name + '.json'), 'utf8'));
const hex = (s) => new Uint8Array(Buffer.from(s || '', 'hex'));
const toHex = (b) => Buffer.from(b).toString('hex');
const text = (s) => new TextDecoder().decode(hex(s));

// modules loads the browser modules the vectors exercise.
function modules() {
  reset();
  load('crypto', 'fileMeta', 'envelope');
  return { gc: window.goneCrypto, env: window.goneEnvelope, meta: window.goneFileMeta };
}

// envelopeCode maps an envelope.js error to its vector error code.
const envelopeCode = (e) => (e.message === 'too many files' ? 'too_many_files' : 'invalid_envelope');

test('AEAD vectors', async (t) => {
  const { gc } = modules();
  const v = readVectors('aead_v1');
  assert.equal(v.aad, 'gone:v1');
  for (const c of v.cases) {
    await t.test(c.name, async (st) => {
      if (c.error) {
        assert.equal(c.error, 'decrypt');
        await assert.rejects(gc.decrypt(hex(c.ciphertext), hex(c.nonce), hex(c.key)));
        return;
      }
      const pt = await gc.decrypt(hex(c.ciphertext), hex(c.nonce), hex(c.key));
      assert.equal(toHex(pt), c.plaintext || '');
      st.mock.method(globalThis.crypto, 'getRandomValues', (arr) => { arr.set(hex(c.nonce)); return arr; });
      const enc = await gc.encrypt(hex(c.plaintext), hex(c.key));
      assert.equal(toHex(enc.ciphertext), c.ciphertext);
    });
  }
});

// randomFeed mocks crypto.getRandomValues to return the given byte arrays
// in order: encryptV2 draws the salt first, then the nonce.
function randomFeed(st, ...chunks) {
  st.mock.method(globalThis.crypto, 'getRandomValues', (arr) => { arr.set(chunks.shift()); return arr; });
}

test('AEAD v2 vectors', async (t) => {
  const { gc } = modules();
  const v = readVectors('aead_v2');
  assert.equal(v.aad, 'gone:v2');
  assert.equal(v.hkdf_info, 'gone:v2 aead key');
  for (const c of v.cases) {
    await t.test(c.name, async (st) => {
      const pass = text(c.passphrase);
      if (c.error) {
        await assert.rejects(gc.decryptV2(hex(c.blob), hex(c.nonce), hex(c.key), pass), (e) => e.code === c.error);
        return;
      }
      const pt = await gc.decryptV2(hex(c.blob), hex(c.nonce), hex(c.key), pass);
      assert.equal(toHex(pt), c.plaintext || '');
      randomFeed(st, hex(c.salt), hex(c.nonce));
      const enc = await gc.encryptV2(hex(c.plaintext), hex(c.key), pass);
      assert.equal(toHex(enc.nonce), c.nonce);
      assert.equal(toHex(enc.ciphertext), c.blob);
    });
  }
});

test('GONE2 pack vectors', async (t) => {
  const { env } = modules();
  for (const c of readVectors('envelope_gone2').pack) {
    await t.test(c.name, () => {
      const files = (c.files || []).map((f) => ({ name: f.name, type: f.type, bytes: hex(f.data) }));
      if (c.error) {
        assert.throws(() => env.encode(c.message, files), (e) => envelopeCode(e) === c.error);
        return;
      }
      const out = env.encode(c.message, files);
      assert.equal(toHex(out), c.expected || '');
      const metas = files.map((f) => ({ name: f.name, type: f.type, size: f.bytes.length }));
      assert.equal(env.encryptedSize(c.message, metas), out.length + 16);
    });
  }
});

test('GONE2 unpack vectors', async (t) => {
  const { env } = modules();
  for (const c of readVectors('envelope_gone2').unpack) {
    await t.test(c.name, () => {
      if (c.error) {
        assert.throws(() => env.decode(hex(c.input)), (e) => envelopeCode(e) === c.error);
        return;
      }
      const out = env.decode(hex(c.input));
      assert.equal(out.message, text(c.message));
      const got = out.files.map((f) => ({ name: f.name, type: f.type, data: toHex(f.bytes) }));
      assert.deepEqual(got, c.files || []);
    });
  }
});

test('fragment vectors', () => {
  const { gc } = modules();
  for (const c of readVectors('fragment_v1').concat(readVectors('fragment_v2'))) {
    if (c.error) {
      assert.throws(() => gc.parseFragment(c.input), (e) => e.code === c.error, c.input);
      continue;
    }
    const f = gc.parseFragment(c.input);
    assert.equal(f.version, c.version, c.input);
    assert.equal(toHex(f.key), c.key, c.input);
  }
});

test('sanitize vectors', () => {
  const { meta } = modules();
  const v = readVectors('sanitize');
  v.names.forEach((c) => assert.equal(meta.sanitizeFileName(c.in), c.out, JSON.stringify(c.in)));
  v.types.forEach((c) => assert.equal(meta.safeType(c.in), c.out, JSON.stringify(c.in)));
});

test('server header vectors match the client checks', () => {
  const { gc } = modules();
  const v = readVectors('server_headers');
  v.versions.forEach((c) => assert.equal(gc.versions.map(String).includes(c.in), c.ok, JSON.stringify(c.in)));
  const nonceOK = (s) => {
    try {
      return gc.b64urlDecode(s).length === 12;
    } catch {
      return false;
    }
  };
  v.nonces.forEach((c) => assert.equal(nonceOK(c.in), c.ok, JSON.stringify(c.in)));
});
