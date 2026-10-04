'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, captureConsole } = require('./harness');
const { FakeXHR } = require('./fakes');

function setup(t) {
  reset('https://gone.test/?debug=timing');
  const logs = captureConsole(t);
  load('util', 'crypto', 'fileMeta', 'envelope', 'submitUpload');
  FakeXHR.reset();
  globalThis.XMLHttpRequest = FakeXHR;
  return { up: window.goneUpload, gc: window.goneCrypto, logs };
}

test('requires crypto, envelope and util; loads once', (t) => {
  reset();
  load('util', 'crypto', 'submitUpload');
  assert.equal(window.goneUpload, undefined);
  const { up } = setup(t);
  load('submitUpload');
  assert.equal(window.goneUpload, up);
});

test('buildPlaintext encodes text-only and file envelopes', async (t) => {
  const { up } = setup(t);
  assert.deepEqual(await up.buildPlaintext('hi', []), new TextEncoder().encode('hi'));
  const out = await up.buildPlaintext('m', [new File(['AB'], 'a.txt', { type: 'text/plain' })]);
  const dec = window.goneEnvelope.decode(out);
  assert.equal(dec.message, 'm');
  assert.equal(dec.files[0].name, 'a.txt');
  assert.deepEqual(Array.from(dec.files[0].bytes), [65, 66]);
});

test('encryptSelection returns a key that decrypts the envelope and logs timing', async (t) => {
  const { up, gc, logs } = setup(t);
  const { keyBytes, encResult } = await up.encryptSelection('secret', []);
  const pt = await gc.decrypt(encResult.ciphertext, encResult.nonce, keyBytes);
  assert.equal(new TextDecoder().decode(pt), 'secret');
  assert.match(logs.log.find((l) => l.includes('encrypt')), /\[gone\]\[timing\] encrypt: /);
});

test('encryptSelection propagates file read failures', async (t) => {
  const { up } = setup(t);
  const bad = { name: 'x', type: '', arrayBuffer: () => Promise.reject(new Error('NotReadableError')) };
  await assert.rejects(up.encryptSelection('', [bad]), /NotReadableError/);
});

test('upload posts ciphertext with headers and reports progress', async (t) => {
  const { up, gc, logs } = setup(t);
  const enc = { nonce: new Uint8Array(12).fill(1), ciphertext: new Uint8Array(40) };
  const progress = [];
  FakeXHR.onSend = (x) => {
    x.upload.onprogress({ lengthComputable: false, loaded: 1, total: 0 });
    x.upload.onprogress({ lengthComputable: true, loaded: 20, total: 40 });
    x.respond(201, { id: 'abc', expires_at: 'soon' });
  };
  const json = await up.upload(enc, '3600', (l, tot) => progress.push([l, tot]));
  assert.deepEqual(json, { id: 'abc', expires_at: 'soon' });
  const x = FakeXHR.instances[0];
  assert.equal(x.method, 'POST');
  assert.equal(x.url, '/api/secret');
  assert.equal(x.body, enc.ciphertext);
  assert.deepEqual(x.headers, {
    'X-Gone-Version': '1', 'X-Gone-Nonce': gc.b64urlEncode(enc.nonce),
    'X-Gone-TTL': '3600', 'Content-Type': 'application/octet-stream'
  });
  assert.deepEqual(progress, [[0, 40], [20, 40]]);
  assert.ok(logs.log.some((l) => l.includes('upload: ')));
});

test('upload accepts 200 and rejects bad statuses or bodies', async (t) => {
  const enc = { nonce: new Uint8Array(12), ciphertext: new Uint8Array(1) };
  const cases = [
    [200, { id: 'x' }, null],
    [400, {}, 'The server rejected the secret'],
    [413, undefined, 'Secret too large for this server'],
    [429, undefined, 'Slow down: too many requests. Please wait and retry.'],
    [503, undefined, 'The server is busy right now. Wait a moment, then try again.'],
    [500, undefined, 'Server error creating secret'],
    [201, undefined, 'Unexpected server response'],
    [201, {}, 'Unexpected server response']
  ];
  for (const [status, body, err] of cases) {
    await t.test(String(status) + ' ' + JSON.stringify(body), async (st) => {
      const { up, logs } = setup(st);
      FakeXHR.onSend = (x) => x.respond(status, body);
      const p = up.upload(enc, '60', () => {});
      if (err) {
        await assert.rejects(p, { message: err });
        if (status >= 400) assert.match(logs.error[0], new RegExp('server error ' + status));
      } else {
        assert.deepEqual(await p, body);
      }
    });
  }
});

test('upload rejects malformed JSON and transport failures', async (t) => {
  const enc = { nonce: new Uint8Array(12), ciphertext: new Uint8Array(1) };
  const { up } = setup(t);
  FakeXHR.onSend = (x) => { x.status = 201; x.responseText = '{oops'; x.onload(); };
  await assert.rejects(up.upload(enc, '60', () => {}), /Unexpected server response/);
  for (const [hook, msg] of [['onerror', 'network error'], ['onabort', 'upload aborted'], ['ontimeout', 'upload timed out']]) {
    FakeXHR.onSend = (x) => x[hook]();
    await assert.rejects(up.upload(enc, '60', () => {}), { message: msg });
  }
});

test('error helpers', (t) => {
  const { up } = setup(t);
  assert.equal(up.uploadErrorMessage(413), 'Secret too large for this server');
  assert.equal(up.uploadErrorMessage(418), 'Server error creating secret');
  const cases = [
    [new Error('network error'), 'Network error uploading secret'],
    [new Error('upload aborted'), 'Network error uploading secret'],
    [new Error('upload timed out'), 'Network error uploading secret'],
    [new Error(''), 'Network error uploading secret'],
    [null, 'Network error uploading secret'],
    [new Error('Secret too large for this server'), 'Secret too large for this server']
  ];
  for (const [e, want] of cases) assert.equal(up.friendlyError(e), want);
});

test('buildShareURL puts the key only in the fragment', (t) => {
  const { up, gc } = setup(t);
  const key = gc.generateKey();
  const url = new URL(up.buildShareURL('abc', key));
  assert.equal(url.origin, 'https://gone.test');
  assert.equal(url.pathname, '/secret/abc');
  assert.equal(url.search, '');
  assert.equal(url.hash, '#v1:' + gc.exportKeyB64(key));
});

test('buildManageURL puts the token only in the fragment and rejects bad input', (t) => {
  const { up } = setup(t);
  const id = '0123456789abcdef0123456789abcdef';
  const token = 'aZ09_-'.repeat(7) + 'x';
  const url = new URL(up.buildManageURL(id, token));
  assert.equal(url.origin, 'https://gone.test');
  assert.equal(url.pathname, '/manage/' + id);
  assert.equal(url.search, '');
  assert.equal(url.hash, '#' + token);
  const cases = [
    [id.toUpperCase(), token], [id + '0', token], ['../x', token], [undefined, token],
    [id, token + 'a'], [id, token.slice(1)], [id, 'A'.repeat(42) + '='], [id, undefined], [id, '']
  ];
  for (const [i, tok] of cases) assert.equal(up.buildManageURL(i, tok), '', `${i} ${tok}`);
});
