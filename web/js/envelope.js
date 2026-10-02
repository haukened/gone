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
(function envelopeModule() {
  if (window.goneEnvelope) return;

  const MAGIC = new Uint8Array([0x47, 0x4f, 0x4e, 0x45, 0x32, 0x00, 0x00, 0x00]);
  const PREFIX_BYTES = MAGIC.length + 4;
  const MAX_HEADER_BYTES = 16 * 1024;
  const MAX_FILES = 10;
  const MAX_NAME_CHARS = 255;
  const GCM_TAG_BYTES = 16;
  const FALLBACK_NAME = 'file';
  const DEFAULT_TYPE = 'application/octet-stream';

  // MIME types that are safe to hand to the browser as-is. Anything else is
  // downloaded as application/octet-stream so it is never rendered inline.
  const SAFE_TYPES = new Set([
    'application/pdf',
    'application/zip',
    'application/gzip',
    'application/json',
    'application/x-tar',
    'application/x-7z-compressed',
    'text/plain',
    'text/csv',
    'image/png',
    'image/jpeg',
    'image/gif',
    'image/webp',
    'audio/mpeg',
    'video/mp4'
  ]);

  // Controls, bidi overrides/isolates and zero-width characters.
  const UNSAFE_CHARS = /[\u0000-\u001f\u007f-\u009f\u200b-\u200f\u202a-\u202e\u2066-\u2069\ufeff]/g;

  function sanitizeFileName(name) {
    const parts = String(name || '').split(/[\\/]+/).filter(Boolean);
    let base = parts.length ? parts[parts.length - 1] : '';
    base = base.replace(UNSAFE_CHARS, '').trim();
    if (!base || base === '.' || base === '..') return FALLBACK_NAME;
    return Array.from(base).slice(0, MAX_NAME_CHARS).join('');
  }

  function safeType(type) {
    const t = String(type || '').toLowerCase().split(';')[0].trim();
    return SAFE_TYPES.has(t) ? t : DEFAULT_TYPE;
  }

  function buildHeader(msgBytes, files) {
    return {
      v: 2,
      msg: msgBytes.length,
      files: files.map(function (f) {
        return { name: sanitizeFileName(f.name), type: safeType(f.type), size: f.bytes.length };
      })
    };
  }

  // headerBytesFor returns the encoded header for a message length and file
  // metadata list without needing the file contents.
  function headerBytesFor(msgLen, fileMetas) {
    const header = {
      v: 2,
      msg: msgLen,
      files: fileMetas.map(function (f) {
        return { name: sanitizeFileName(f.name), type: safeType(f.type), size: f.size };
      })
    };
    return new TextEncoder().encode(JSON.stringify(header));
  }

  // encryptedSize returns the ciphertext size (including the GCM tag) the
  // server will see for the given message and file metadata.
  function encryptedSize(message, fileMetas) {
    const msgLen = new TextEncoder().encode(message || '').length;
    if (!fileMetas.length) return msgLen + GCM_TAG_BYTES;
    const header = headerBytesFor(msgLen, fileMetas);
    const fileTotal = fileMetas.reduce(function (sum, f) { return sum + f.size; }, 0);
    return PREFIX_BYTES + header.length + msgLen + fileTotal + GCM_TAG_BYTES;
  }

  function writeU32(buf, offset, n) {
    buf[offset] = (n >>> 24) & 0xff;
    buf[offset + 1] = (n >>> 16) & 0xff;
    buf[offset + 2] = (n >>> 8) & 0xff;
    buf[offset + 3] = n & 0xff;
  }

  function readU32(buf, offset) {
    return ((buf[offset] << 24) | (buf[offset + 1] << 16) | (buf[offset + 2] << 8) | buf[offset + 3]) >>> 0;
  }

  // encode builds the plaintext. files is [{name, type, bytes: Uint8Array}].
  // A text-only secret is encoded as raw UTF-8 for compatibility.
  function encode(message, files) {
    const msgBytes = new TextEncoder().encode(message || '');
    if (!files || !files.length) return msgBytes;
    if (files.length > MAX_FILES) throw new Error('too many files');
    const header = new TextEncoder().encode(JSON.stringify(buildHeader(msgBytes, files)));
    if (header.length > MAX_HEADER_BYTES) throw new Error('header too large');
    const total = files.reduce(function (sum, f) { return sum + f.bytes.length; }, PREFIX_BYTES + header.length + msgBytes.length);
    const out = new Uint8Array(total);
    out.set(MAGIC, 0);
    writeU32(out, MAGIC.length, header.length);
    let offset = PREFIX_BYTES;
    out.set(header, offset);
    offset += header.length;
    out.set(msgBytes, offset);
    offset += msgBytes.length;
    files.forEach(function (f) {
      out.set(f.bytes, offset);
      offset += f.bytes.length;
    });
    msgBytes.fill(0);
    return out;
  }

  function hasMagic(bytes) {
    if (!bytes || bytes.length < MAGIC.length) return false;
    for (let i = 0; i < MAGIC.length; i++) {
      if (bytes[i] !== MAGIC[i]) return false;
    }
    return true;
  }

  function isSize(n) {
    return Number.isSafeInteger(n) && n >= 0;
  }

  function validateHeader(h, available) {
    if (!h || h.v !== 2 || !isSize(h.msg) || !Array.isArray(h.files)) throw new Error('invalid envelope header');
    if (h.files.length > MAX_FILES) throw new Error('too many files');
    let sum = h.msg;
    h.files.forEach(function (f) {
      if (!f || !isSize(f.size)) throw new Error('invalid file entry');
      sum += f.size;
    });
    if (sum !== available) throw new Error('envelope size mismatch');
  }

  function readHeader(bytes) {
    if (bytes.length < PREFIX_BYTES) throw new Error('truncated envelope');
    const headerLen = readU32(bytes, MAGIC.length);
    if (headerLen > MAX_HEADER_BYTES || headerLen > bytes.length - PREFIX_BYTES) throw new Error('invalid header length');
    const raw = bytes.subarray(PREFIX_BYTES, PREFIX_BYTES + headerLen);
    const header = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(raw));
    const bodyStart = PREFIX_BYTES + headerLen;
    validateHeader(header, bytes.length - bodyStart);
    return { header: header, bodyStart: bodyStart };
  }

  // decode parses plaintext into {message, files:[{name, type, size, bytes}]}.
  // File bytes are views into the input buffer; callers own the buffer.
  function decode(bytes) {
    if (!hasMagic(bytes)) {
      return { message: new TextDecoder().decode(bytes), files: [] };
    }
    const parsed = readHeader(bytes);
    let offset = parsed.bodyStart;
    const message = new TextDecoder().decode(bytes.subarray(offset, offset + parsed.header.msg));
    offset += parsed.header.msg;
    const files = parsed.header.files.map(function (f) {
      const view = bytes.subarray(offset, offset + f.size);
      offset += f.size;
      return { name: sanitizeFileName(f.name), type: safeType(f.type), size: f.size, bytes: view };
    });
    return { message: message, files: files };
  }

  function formatBytes(bytes) {
    if (!Number.isFinite(bytes) || bytes < 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
      value /= 1024;
      unit++;
    }
    const digits = unit === 0 ? 0 : value < 10 ? 1 : 0;
    return `${value.toFixed(digits)} ${units[unit]}`;
  }

  window.goneEnvelope = Object.freeze({
    MAX_FILES: MAX_FILES,
    encode: encode,
    decode: decode,
    encryptedSize: encryptedSize,
    sanitizeFileName: sanitizeFileName,
    safeType: safeType,
    formatBytes: formatBytes
  });
})();
