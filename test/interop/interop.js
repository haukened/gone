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
//   {"op":"sealReply","key","message","files"}   key is the base64url pubR
//     -> {"nonce","body"}                         protocol v3 (reply page)
//   {"op":"requestKey"} -> {"key","private"}      private is a JWK (JSON)
//   {"op":"openReply","key","private","nonce","body"}
//     -> {"message","files":[...]}                protocol v3 (request page)

const { reset, load } = require('../js/harness');

const b64 = (bytes) => Buffer.from(bytes).toString('base64');
const bytes = (s) => new Uint8Array(Buffer.from(s || '', 'base64'));

// modules installs the fake browser and loads the sealing modules. Module
// load banners go to stderr so stdout carries only the reply.
function modules() {
  console.log = (...args) => process.stderr.write(args.join(' ') + '\n');
  reset();
  load('crypto', 'cryptoV3', 'fileMeta', 'envelope');
  return { gc: window.goneCrypto, v3: window.goneCryptoV3, env: window.goneEnvelope };
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
  return decoded(m, pt);
}

// decoded returns the bridge reply for decrypted plaintext.
function decoded(m, pt) {
  const p = m.env.decode(new Uint8Array(pt));
  return { message: p.message, files: p.files.map((f) => ({ name: f.name, type: f.type, data: b64(f.bytes) })) };
}

// sealReply encrypts req to a requester's public key as the reply page does.
async function sealReply(m, req) {
  const files = (req.files || []).map((f) => ({ name: f.name, type: f.type, bytes: bytes(f.data) }));
  const plaintext = m.env.encode(req.message || '', files);
  const out = await m.v3.encryptV3(plaintext, m.gc.b64urlDecode(req.key));
  return { nonce: b64(out.nonce), body: b64(out.ciphertext) };
}

// requestKey makes a requester key pair. The browser keeps its private key
// non-extractable in IndexedDB; this one-shot bridge has no such store, so
// the test key is exported as a JWK and re-imported by openReply.
async function requestKey(m) {
  const curve = { name: 'ECDH', namedCurve: 'P-256' };
  const kp = await crypto.subtle.generateKey(curve, true, ['deriveBits']);
  const pub = new Uint8Array(await crypto.subtle.exportKey('raw', kp.publicKey));
  return { key: m.gc.b64urlEncode(pub), private: JSON.stringify(await crypto.subtle.exportKey('jwk', kp.privateKey)) };
}

// openReply decrypts and decodes a v3 reply as the request page does.
async function openReply(m, req) {
  const curve = { name: 'ECDH', namedCurve: 'P-256' };
  const priv = await crypto.subtle.importKey('jwk', JSON.parse(req.private), curve, false, ['deriveBits']);
  return decoded(m, await m.v3.decryptV3(bytes(req.body), bytes(req.nonce), priv, m.gc.b64urlDecode(req.key)));
}

const OPS = { seal: seal, open: open, sealReply: sealReply, requestKey: requestKey, openReply: openReply };

// main dispatches one request read from stdin.
async function main() {
  const chunks = [];
  for await (const c of process.stdin) chunks.push(c);
  const req = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  const m = modules();
  const op = Object.hasOwn(OPS, req.op) ? OPS[req.op] : null;
  if (!op) throw new Error('unknown op');
  const result = await op(m, req);
  process.stdout.write(JSON.stringify(result));
}

main().catch((e) => {
  process.stderr.write(String((e && e.code) || (e && e.message) || e) + '\n');
  process.exit(1);
});
