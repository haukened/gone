'use strict';

// Envelope v2 framing: the magic prefix, the big-endian header length and
// the JSON header that lists the message and file sizes. envelope.js builds
// and splits whole envelopes on top of this. The normative format is
// docs/protocol.md section 5. Exposed as window.goneEnvelopeHeader.
(function envelopeHeaderModule() {
  if (window.goneEnvelopeHeader) return;

  const MAGIC = new Uint8Array([0x47, 0x4f, 0x4e, 0x45, 0x32, 0x00, 0x00, 0x00]);
  const PREFIX_BYTES = MAGIC.length + 4;
  const MAX_HEADER_BYTES = 16 * 1024;
  const MAX_FILES = 10;

  function sumSizes(entries, start) {
    return entries.reduce(function (sum, f) { return sum + f.size; }, start);
  }

  // Big-endian u32 helpers (DataView defaults to big-endian).
  function viewOf(buf) {
    return new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  }

  function hasMagic(bytes) {
    return Boolean(bytes) && bytes.length >= MAGIC.length && MAGIC.every(function (b, i) { return bytes[i] === b; });
  }

  function isSize(n) {
    return Number.isSafeInteger(n) && n >= 0;
  }

  function isObject(v) {
    return typeof v === 'object' && v !== null && !Array.isArray(v);
  }

  function isValidFile(f) {
    return isObject(f) && typeof f.name === 'string' && typeof f.type === 'string' && isSize(f.size);
  }

  function isValidShape(h) {
    return isObject(h) && h.v === 2 && isSize(h.msg) && Array.isArray(h.files);
  }

  // validateHeader checks the header shape and that its sizes exactly
  // account for the available body bytes.
  function validateHeader(h, available) {
    if (!isValidShape(h)) throw new Error('invalid envelope header');
    if (h.files.length > MAX_FILES) throw new Error('too many files');
    if (!h.files.every(isValidFile)) throw new Error('invalid file entry');
    if (sumSizes(h.files, h.msg) !== available) throw new Error('envelope size mismatch');
  }

  function readHeader(bytes) {
    if (bytes.length < PREFIX_BYTES) throw new Error('truncated envelope');
    const headerLen = viewOf(bytes).getUint32(MAGIC.length);
    if (headerLen > Math.min(MAX_HEADER_BYTES, bytes.length - PREFIX_BYTES)) throw new Error('invalid header length');
    const bodyStart = PREFIX_BYTES + headerLen;
    const header = JSON.parse(new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes.subarray(PREFIX_BYTES, bodyStart)));
    validateHeader(header, bytes.length - bodyStart);
    return { header: header, bodyStart: bodyStart };
  }

  window.goneEnvelopeHeader = Object.freeze({
    MAGIC: MAGIC,
    PREFIX_BYTES: PREFIX_BYTES,
    MAX_HEADER_BYTES: MAX_HEADER_BYTES,
    MAX_FILES: MAX_FILES,
    sumSizes: sumSizes,
    viewOf: viewOf,
    hasMagic: hasMagic,
    readHeader: readHeader
  });
})();
