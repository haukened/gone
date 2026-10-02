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

  // Controls, bidi overrides/isolates and zero-width characters.
  const UNSAFE_RANGES = [
    [0x00, 0x1f], [0x7f, 0x9f], [0x200b, 0x200f],
    [0x202a, 0x202e], [0x2066, 0x2069], [0xfeff, 0xfeff]
  ];

  function isSafeChar(ch) {
    const cp = ch.codePointAt(0);
    return !UNSAFE_RANGES.some(function (r) { return cp >= r[0] && cp <= r[1]; });
  }

  // sanitizeFileName keeps only the last path segment, strips unsafe
  // characters, and caps the length; empty or dot names become "file".
  function sanitizeFileName(name) {
    const parts = String(name || '').split(/[\\/]+/).filter(Boolean);
    const last = parts.length ? parts[parts.length - 1] : '';
    const base = Array.from(last).filter(isSafeChar).join('').trim();
    if (base === '' || base === '.' || base === '..') return FALLBACK_NAME;
    return Array.from(base).slice(0, MAX_NAME_CHARS).join('');
  }

  // safeType returns type when allowlisted, otherwise the generic binary type.
  function safeType(type) {
    const t = String(type || '').toLowerCase().split(';')[0].trim();
    return SAFE_TYPES.has(t) ? t : DEFAULT_TYPE;
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
