'use strict';

// QR code encoder (ISO/IEC 18004) for the share link, so the result page can
// draw a code without a third-party library. It supports only what a link
// needs: byte mode, error correction level M, versions 1 to 40, and automatic
// mask selection. The structure follows Project Nayuki's reference
// implementation. Exposed as window.goneQr.
(function qrModule() {
  if (window.goneQr) return;
  const codewords = window.goneQrCodewords;
  const grid = window.goneQrGrid;
  const patterns = window.goneQrPatterns;
  const scoring = window.goneQrPenalty;
  if (!codewords || !grid || !patterns || !scoring) return;

  const MASKS = [
    (x, y) => (x + y) % 2 === 0,
    (x, y) => y % 2 === 0,
    (x) => x % 3 === 0,
    (x, y) => (x + y) % 3 === 0,
    (x, y) => (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0,
    (x, y) => ((x * y) % 2) + ((x * y) % 3) === 0,
    (x, y) => (((x * y) % 2) + ((x * y) % 3)) % 2 === 0,
    (x, y) => (((x + y) % 2) + ((x * y) % 3)) % 2 === 0
  ];

  // applyMask XORs the mask into every data module; applying it twice undoes it.
  function applyMask(g, mask) {
    const fn = MASKS[mask];
    for (let y = 0; y < g.size; y++) {
      for (let x = 0; x < g.size; x++) {
        if (!g.fixed[y * g.size + x] && fn(x, y)) g.dark[y * g.size + x] ^= 1;
      }
    }
  }

  // chooseMask applies each mask in turn and keeps the lowest penalty.
  function chooseMask(g) {
    let best = 0;
    let bestScore = Infinity;
    for (let mask = 0; mask < 8; mask++) {
      applyMask(g, mask);
      grid.drawFormat(g, mask);
      const score = scoring.penalty(g);
      if (score < bestScore) {
        best = mask;
        bestScore = score;
      }
      applyMask(g, mask);
    }
    return best;
  }

  // encode returns {version, size, mask, dark} for text, where dark is a
  // row-major Uint8Array of size*size modules (1 = dark), quiet zone not
  // included. opts.mask (0-7) forces a mask; it exists for tests.
  function encode(text, opts) {
    const bytes = Array.from(new TextEncoder().encode(String(text)));
    const ver = codewords.pickVersion(bytes.length);
    const g = grid.newGrid(ver * 4 + 17);
    patterns.drawFunctionPatterns(g, ver);
    grid.drawCodewords(g, codewords.interleave(codewords.dataBytes(bytes, ver), ver));
    const forced = opts && opts.mask;
    const mask = Number.isInteger(forced) && forced >= 0 && forced < 8 ? forced : chooseMask(g);
    applyMask(g, mask);
    grid.drawFormat(g, mask);
    return { version: ver, size: g.size, mask: mask, dark: g.dark };
  }

  window.goneQr = Object.freeze({ encode: encode });
})();
