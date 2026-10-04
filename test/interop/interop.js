/* global Buffer, process */
'use strict';

// Interop bridge for test/cli_interop_test.go. It runs the browser's own
// crypto.js, fileMeta.js and envelope.js under the node test harness so the
// Go CLI can be checked against the exact code the web UI ships.
//
// Reads one JSON request on stdin and writes one JSON reply on stdout.
// Binary fields are standard base64, matching Go's encoding of []byte.
//
//   {"op":"seal","message":"..","files":[{"name","type","data"}],"passphrase":".."}
//     -> {"version","key","nonce","body"}     key is the base64url link key
//   {"op":"open","version","key","nonce","body","passphrase"}
//     -> {"message","files":[{"name","type","data"}]}

const { reset, load } = require('../js/harness');

const b64 = (bytes) => Buffer.from(bytes).toString('base64');
const bytes = (s) => new Uint8Array(Buffer.from(s || '', 'base64'));

// modules installs the fake browser and loads the sealing modules. Module
// load banners go to stderr so stdout carries only the reply.
function modules() {
  console.log = (...args) => process.stderr.write(args.join(' ') + '\n');
  reset();
  load('crypto', 'fileMeta', 'envelope');
  return { gc: window.goneCrypto, env: window.goneEnvelope };
}

// seal encodes and encrypts req as the browser's submit flow does: v2 when a
// passphrase is set, v1 otherwise.
async function seal(m, req) {
  const files = (req.files || []).map((f) => ({ name: f.name, type: f.type, bytes: bytes(f.data) }));
  const plaintext = m.env.encode(req.message || '', files);
  const key = m.gc.generateKey();
  const out = req.passphrase
    ? await m.gc.encryptV2(plaintext, key, req.passphrase)
    : await m.gc.encrypt(plaintext, key);
  return {
    version: req.passphrase ? m.gc.versionV2 : m.gc.version,
    key: m.gc.exportKeyB64(key),
    nonce: b64(out.nonce),
    body: b64(out.ciphertext)
  };
}

// open decrypts and decodes req as the browser's consume flow does.
async function open(m, req) {
  const frag = m.gc.parseFragment('v' + req.version + ':' + req.key);
  const pt = frag.version === m.gc.versionV2
    ? await m.gc.decryptV2(bytes(req.body), bytes(req.nonce), frag.key, req.passphrase)
    : await m.gc.decrypt(bytes(req.body), bytes(req.nonce), frag.key);
  const p = m.env.decode(new Uint8Array(pt));
  return { message: p.message, files: p.files.map((f) => ({ name: f.name, type: f.type, data: b64(f.bytes) })) };
}

// main dispatches one request read from stdin.
async function main() {
  const chunks = [];
  for await (const c of process.stdin) chunks.push(c);
  const req = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  const m = modules();
  let result;
  switch (req.op) {
    case 'seal':
      result = await seal(m, req);
      break;
    case 'open':
      result = await open(m, req);
      break;
    default:
      throw new Error('unknown op');
  }
  process.stdout.write(JSON.stringify(result));
}

main().catch((e) => {
  process.stderr.write(String((e && e.code) || (e && e.message) || e) + '\n');
  process.exit(1);
});
