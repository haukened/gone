'use strict';

// Protocol v3 (docs/protocol.md section 4.3): replies encrypted to a
// requester's ECDH P-256 public key. A secret link never uses v3, so
// goneCrypto.parseFragment keeps rejecting it; reply links have their own
// parser here. Exposed as window.goneCryptoV3.
(function cryptoV3Module() {
  if (window.goneCryptoV3 || !window.goneCryptoCore || !window.goneCryptoEncoding || !window.goneCryptoV2Key) return;
  const core = window.goneCryptoCore;
  const enc = window.goneCryptoEncoding;
  const concat = window.goneCryptoV2Key.concat;
  const VERSION = 3;
  const PUBLIC_KEY_BYTES = 65;
  const TAG_BYTES = 16;
  const AAD_V3 = 'gone:v3';
  const HKDF_INFO_V3 = 'gone:v3 aead key';
  const MAX_FRAGMENT_CHARS = 512;
  const CURVE = { name: 'ECDH', namedCurve: 'P-256' };
  // Anchored, bounded character classes: linear time.
  const FRAGMENT_RE = /^v([1-9][0-9]{0,2}):([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]*)$/;
  const FILL_RE = /^[A-Za-z0-9_-]{43}$/;

  const utf8 = (s) => new TextEncoder().encode(s);

  // generateKeyPair makes the requester's key pair. The private key is
  // non-extractable: it can be stored in IndexedDB and used to decrypt, but
  // its bytes can never be read back by script.
  //
  // Returns {privateKey, publicKey} where publicKey is the 65-byte raw point.
  async function generateKeyPair() {
    const kp = await crypto.subtle.generateKey(CURVE, false, ['deriveBits']);
    const pub = new Uint8Array(await crypto.subtle.exportKey('raw', kp.publicKey));
    return { privateKey: kp.privateKey, publicKey: pub };
  }

  function importPublic(raw) {
    return crypto.subtle.importKey('raw', raw, CURVE, true, []);
  }

  // aeadKey runs ECDH and HKDF-SHA-256 into a non-extractable AES-GCM key:
  // HKDF(Z, salt = empty, info = "gone:v3 aead key" || pubE || pubR).
  async function aeadKey(privateKey, peer, pubE, pubR, usage) {
    const z = new Uint8Array(await crypto.subtle.deriveBits({ name: 'ECDH', public: peer }, privateKey, 256));
    const hkdf = await crypto.subtle.importKey('raw', z, 'HKDF', false, ['deriveKey']);
    z.fill(0);
    const info = concat(concat(utf8(HKDF_INFO_V3), pubE), pubR);
    const params = { name: 'HKDF', hash: 'SHA-256', salt: new Uint8Array(0), info: info };
    return crypto.subtle.deriveKey(params, hkdf, { name: 'AES-GCM', length: 256 }, false, [usage]);
  }

  function aad(header) {
    return concat(utf8(AAD_V3), header);
  }

  // sealWith encrypts data to pubR with the given ephemeral key pair and
  // nonce. encryptV3 supplies fresh ones; the vectors supply fixed ones.
  async function sealWith(data, pubR, eph, nonce) {
    const pt = core.toBytes(data);
    const peer = await importPublic(pubR).catch(() => { throw core.codedError('invalid_key'); });
    const pubE = new Uint8Array(await crypto.subtle.exportKey('raw', eph.publicKey));
    const key = await aeadKey(eph.privateKey, peer, pubE, pubR, 'encrypt');
    const ct = await crypto.subtle.encrypt(core.gcmParams(nonce, aad(pubE)), key, pt);
    return { nonce: nonce, ciphertext: concat(pubE, new Uint8Array(ct)) };
  }

  // encryptV3 seals data to the requester's 65-byte public key with a fresh
  // ephemeral key and nonce. Rejects with code "invalid_key" when pubR is not
  // a point on the curve.
  //
  // Returns {nonce, ciphertext} where ciphertext = pubE || ct || tag.
  async function encryptV3(data, pubR) {
    const eph = await crypto.subtle.generateKey(CURVE, true, ['deriveBits']);
    return sealWith(data, pubR, eph, core.randomBytes(core.NONCE_BYTES));
  }

  // decryptV3 opens a reply with the requester's private key. pubR is the
  // matching public key, which the requester keeps next to it. Every failure
  // rejects with the single code "decrypt".
  async function decryptV3(blob, nonce, privateKey, pubR) {
    if (!blob || blob.length < PUBLIC_KEY_BYTES + TAG_BYTES || !nonce || nonce.length !== core.NONCE_BYTES) {
      throw core.codedError('decrypt');
    }
    const pubE = blob.subarray(0, PUBLIC_KEY_BYTES);
    try {
      const peer = await importPublic(pubE);
      const key = await aeadKey(privateKey, peer, pubE, pubR, 'decrypt');
      return new Uint8Array(await crypto.subtle.decrypt(core.gcmParams(nonce, aad(pubE)), key, blob.subarray(PUBLIC_KEY_BYTES)));
    } catch {
      throw core.codedError('decrypt');
    }
  }

  function fragmentParts(s) {
    const m = typeof s === 'string' && s.length <= MAX_FRAGMENT_CHARS ? FRAGMENT_RE.exec(s) : null;
    if (!m) throw core.codedError('invalid_fragment');
    if (Number(m[1]) !== VERSION) throw core.codedError('unsupported_version');
    return m;
  }

  function decodePublicKey(b64) {
    let key = null;
    try {
      key = enc.b64urlDecode(b64);
    } catch {
      key = null;
    }
    if (!key || key.length !== PUBLIC_KEY_BYTES || key[0] !== 0x04) throw core.codedError('invalid_fragment');
    return key;
  }

  // parseReplyFragment parses "v3:<pubkey>.<fill>" (section 7.4) into
  // {publicKey, fill}. Whether the key is on the curve is checked when
  // sealing.
  function parseReplyFragment(s) {
    const m = fragmentParts(s);
    const publicKey = decodePublicKey(m[2]);
    if (!FILL_RE.test(m[3])) throw core.codedError('invalid_fragment');
    return { publicKey: publicKey, fill: m[3] };
  }

  // replyFragment renders the fragment for a public key and fill token.
  function replyFragment(publicKey, fill) {
    return `v${VERSION}:${enc.b64urlEncode(publicKey)}.${fill}`;
  }

  window.goneCryptoV3 = Object.freeze({
    version: VERSION,
    headerBytes: PUBLIC_KEY_BYTES,
    generateKeyPair: generateKeyPair,
    encryptV3: encryptV3,
    decryptV3: decryptV3,
    sealWith: sealWith,
    parseReplyFragment: parseReplyFragment,
    replyFragment: replyFragment
  });
})();
