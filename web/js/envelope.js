'use strict';

// Envelope v2: packs an optional message and up to MAX_FILES files into one
// plaintext buffer that is encrypted as a single AES-GCM blob.
//
// Layout:
//   magic "GONE2\0\0\0" (8 bytes)
//   header length (u32, big-endian)
//   header JSON {"v":2,"msg":<bytes>,"files":[{"name","type","size"}]}
//   message bytes (UTF-8)
//   file bytes, concatenated in header order
//
// Plaintext without the magic prefix is treated as a legacy text-only secret.
// The normative format is docs/protocol.md section 5.
// Requires window.goneFileMeta (fileMeta.js) and window.goneEnvelopeHeader
// (envelopeHeader.js).
(function envelopeModule() {
  if (window.goneEnvelope || !window.goneFileMeta || !window.goneEnvelopeHeader) return;
  const meta = window.goneFileMeta;
  const framing = window.goneEnvelopeHeader;
  const MAGIC = framing.MAGIC;
  const PREFIX_BYTES = framing.PREFIX_BYTES;
  const MAX_HEADER_BYTES = framing.MAX_HEADER_BYTES;
  const MAX_FILES = framing.MAX_FILES;
  const GCM_TAG_BYTES = 16;
  const sumSizes = framing.sumSizes;
  const viewOf = framing.viewOf;
  const hasMagic = framing.hasMagic;
  const readHeader = framing.readHeader;

  function utf8(s) {
    return new TextEncoder().encode(s || '');
  }

  // headerBytes encodes the JSON header for a message length and file
  // metadata list ({name, type, size}); names and types are sanitized.
  function headerBytes(msgLen, fileMetas) {
    const files = fileMetas.map(function (f) {
      return { name: meta.sanitizeFileName(f.name), type: meta.safeType(f.type), size: f.size };
    });
    return utf8(JSON.stringify({ v: 2, msg: msgLen, files: files }));
  }

  // needsEnvelope reports whether the plaintext must be GONE2: there are
  // files, or the text itself starts with the magic and would be misread.
  function needsEnvelope(msgBytes, fileCount) {
    return fileCount > 0 || hasMagic(msgBytes);
  }

  // encryptedSize returns the ciphertext size (including the GCM tag) the
  // server will see for the given message and file metadata. overhead is any
  // protocol prefix on the blob, such as the 21-byte v2 header (default 0).
  function encryptedSize(message, fileMetas, overhead) {
    const msgBytes = utf8(message);
    const fixed = GCM_TAG_BYTES + (overhead || 0);
    const msgLen = msgBytes.length;
    if (!needsEnvelope(msgBytes, fileMetas.length)) return msgLen + fixed;
    const header = headerBytes(msgLen, fileMetas);
    return sumSizes(fileMetas, PREFIX_BYTES + header.length + msgLen + fixed);
  }

  // concatParts writes each part sequentially into a new buffer of length total.
  function concatParts(parts, total) {
    const out = new Uint8Array(total);
    let offset = 0;
    parts.forEach(function (p) {
      out.set(p, offset);
      offset += p.length;
    });
    return out;
  }

  // encode builds the plaintext. files is [{name, type, bytes: Uint8Array}].
  // A text-only secret is raw UTF-8 unless it starts with the magic.
  function encode(message, files) {
    const list = files || [];
    const msgBytes = utf8(message);
    if (list.length > MAX_FILES) throw new Error('too many files');
    if (!needsEnvelope(msgBytes, list.length)) return msgBytes;
    const metas = list.map(function (f) { return { name: f.name, type: f.type, size: f.bytes.length }; });
    const header = headerBytes(msgBytes.length, metas);
    if (header.length > MAX_HEADER_BYTES) throw new Error('header too large');
    const lenField = new Uint8Array(4);
    viewOf(lenField).setUint32(0, header.length);
    const parts = [MAGIC, lenField, header, msgBytes].concat(list.map(function (f) { return f.bytes; }));
    const out = concatParts(parts, sumSizes(metas, PREFIX_BYTES + header.length + msgBytes.length));
    msgBytes.fill(0);
    return out;
  }

  // decode parses plaintext into {message, files:[{name, type, size, bytes}]}.
  // File bytes are views into the input buffer; callers own the buffer.
  function decode(bytes) {
    if (!hasMagic(bytes)) {
      return { message: new TextDecoder().decode(bytes), files: [] };
    }
    const parsed = readHeader(bytes);
    let offset = parsed.bodyStart + parsed.header.msg;
    const message = new TextDecoder().decode(bytes.subarray(parsed.bodyStart, offset));
    const files = parsed.header.files.map(function (f) {
      const view = bytes.subarray(offset, offset + f.size);
      offset += f.size;
      return { name: meta.sanitizeFileName(f.name), type: meta.safeType(f.type), size: f.size, bytes: view };
    });
    return { message: message, files: files };
  }

  window.goneEnvelope = Object.freeze({
    MAX_FILES: MAX_FILES,
    encode: encode,
    decode: decode,
    encryptedSize: encryptedSize,
    sanitizeFileName: meta.sanitizeFileName,
    safeType: meta.safeType,
    formatBytes: meta.formatBytes
  });
})();
