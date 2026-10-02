'use strict';

// File metadata helpers shared by the envelope and the UI: safe display
// names, a MIME type allowlist, and human-readable sizes. Exposed as
// window.goneFileMeta.
(function fileMetaModule() {
  if (window.goneFileMeta) return;

  const MAX_NAME_CHARS = 255;
  const FALLBACK_NAME = 'file';
  const DEFAULT_TYPE = 'application/octet-stream';
  const UNITS = ['B', 'KB', 'MB', 'GB'];

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

  // Controls, invisible bidi marks, overrides/isolates and zero-width
  // characters (docs/protocol.md section 6.1).
  const UNSAFE_RANGES = [
    [0x00, 0x1f], [0x7f, 0x9f], [0x061c, 0x061c], [0x180e, 0x180e],
    [0x200b, 0x200f], [0x202a, 0x202e], [0x2060, 0x206f], [0xfeff, 0xfeff]
  ];

  function isASCII(s) {
    for (let i = 0; i < s.length; i++) {
      if (s.charCodeAt(i) > 0x7f) return false;
    }
    return true;
  }

  function isUnsafe(cp) {
    return UNSAFE_RANGES.some(function (r) { return cp >= r[0] && cp <= r[1]; });
  }

  // safeChars drops unsafe characters, turns lone surrogates into U+FFFD
  // and keeps at most MAX_NAME_CHARS code points.
  function safeChars(s) {
    const out = [];
    for (const ch of s) {
      if (out.length === MAX_NAME_CHARS) break;
      const cp = ch.codePointAt(0);
      if (isUnsafe(cp)) continue;
      out.push(cp >= 0xd800 && cp <= 0xdfff ? '\ufffd' : ch);
    }
    return out.join('');
  }

  // sanitizeFileName keeps only the last path segment, strips unsafe
  // characters, caps the length and trims whitespace; empty or dot names
  // become "file". Must match internal/envelope.SanitizeFileName.
  function sanitizeFileName(name) {
    const parts = String(name || '').split(/[\\/]+/).filter(Boolean);
    const last = parts.length ? parts[parts.length - 1] : '';
    const base = safeChars(last).trim();
    if (base === '' || base === '.' || base === '..') return FALLBACK_NAME;
    return base;
  }

  // safeType returns the type's essence (before ";", trimmed, lowercased)
  // when allowlisted, otherwise the generic binary type. Non-ASCII input is
  // rejected before lowercasing so Unicode case folding cannot forge a match.
  function safeType(type) {
    const t = String(type || '').split(';')[0].trim();
    if (!isASCII(t)) return DEFAULT_TYPE;
    const lower = t.toLowerCase();
    return SAFE_TYPES.has(lower) ? lower : DEFAULT_TYPE;
  }

  // formatBytes renders a byte count as B/KB/MB/GB with at most one decimal.
  function formatBytes(bytes) {
    if (!Number.isFinite(bytes) || bytes < 0) return '0 B';
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < UNITS.length - 1) {
      value /= 1024;
      unit++;
    }
    const digits = unit > 0 && value < 10 ? 1 : 0;
    return `${value.toFixed(digits)} ${UNITS[unit]}`;
  }

  window.goneFileMeta = Object.freeze({
    sanitizeFileName: sanitizeFileName,
    safeType: safeType,
    formatBytes: formatBytes
  });
})();
