'use strict';

// Module grid for the QR encoder (window.goneQr): the grid itself, format
// and version information, and the data codeword zigzag. Exposed as
// window.goneQrGrid.
(function qrGridModule() {
  if (window.goneQrGrid) return;

  const FORMAT_BITS_M = 0;

  function newGrid(size) {
    return { size: size, dark: new Uint8Array(size * size), fixed: new Uint8Array(size * size) };
  }

  function setFixed(g, x, y, dark) {
    g.dark[y * g.size + x] = dark ? 1 : 0;
    g.fixed[y * g.size + x] = 1;
  }

  function bit(value, i) {
    return ((value >>> i) & 1) !== 0;
  }

  function drawFormat(g, mask) {
    const size = g.size;
    const data = (FORMAT_BITS_M << 3) | mask;
    let rem = data;
    for (let i = 0; i < 10; i++) rem = (rem << 1) ^ ((rem >>> 9) * 0x537);
    const bits = ((data << 10) | rem) ^ 0x5412;
    for (let i = 0; i <= 5; i++) setFixed(g, 8, i, bit(bits, i));
    setFixed(g, 8, 7, bit(bits, 6));
    setFixed(g, 8, 8, bit(bits, 7));
    setFixed(g, 7, 8, bit(bits, 8));
    for (let i = 9; i < 15; i++) setFixed(g, 14 - i, 8, bit(bits, i));
    for (let i = 0; i < 8; i++) setFixed(g, size - 1 - i, 8, bit(bits, i));
    for (let i = 8; i < 15; i++) setFixed(g, 8, size - 15 + i, bit(bits, i));
    setFixed(g, 8, size - 8, true);
  }

  function drawVersion(g, ver) {
    if (ver < 7) return;
    let rem = ver;
    for (let i = 0; i < 12; i++) rem = (rem << 1) ^ ((rem >>> 11) * 0x1f25);
    const bits = (ver << 12) | rem;
    for (let i = 0; i < 18; i++) {
      const a = g.size - 11 + (i % 3);
      const b = Math.floor(i / 3);
      setFixed(g, a, b, bit(bits, i));
      setFixed(g, b, a, bit(bits, i));
    }
  }

  // drawCodewords places the data bits in the two-column zigzag, skipping
  // function modules. Leftover remainder bits stay light.
  function drawCodewords(g, codewords) {
    const size = g.size;
    const total = codewords.length * 8;
    let i = 0;
    for (let right = size - 1; right >= 1; right -= 2) {
      if (right === 6) right = 5;
      const upward = ((right + 1) & 2) === 0;
      for (let vert = 0; vert < size; vert++) {
        const y = upward ? size - 1 - vert : vert;
        for (let x = right; x > right - 2; x--) {
          if (g.fixed[y * size + x] || i >= total) continue;
          g.dark[y * size + x] = (codewords[i >>> 3] >>> (7 - (i & 7))) & 1;
          i++;
        }
      }
    }
  }

  window.goneQrGrid = Object.freeze({
    newGrid: newGrid,
    setFixed: setFixed,
    drawFormat: drawFormat,
    drawVersion: drawVersion,
    drawCodewords: drawCodewords
  });
})();
