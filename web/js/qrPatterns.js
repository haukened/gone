'use strict';

// Function patterns for the QR encoder (window.goneQr): timing lines,
// finders, alignment patterns, and the reserved format and version areas.
// Exposed as window.goneQrPatterns.
(function qrPatternsModule() {
  if (window.goneQrPatterns || !window.goneQrGrid) return;
  const grid = window.goneQrGrid;
  const setFixed = grid.setFixed;

  function drawFinder(g, cx, cy) {
    for (let dy = -4; dy <= 4; dy++) {
      for (let dx = -4; dx <= 4; dx++) {
        const x = cx + dx;
        const y = cy + dy;
        const dist = Math.max(Math.abs(dx), Math.abs(dy));
        if (x >= 0 && x < g.size && y >= 0 && y < g.size) setFixed(g, x, y, dist !== 2 && dist !== 4);
      }
    }
  }

  function drawAlignment(g, cx, cy) {
    for (let dy = -2; dy <= 2; dy++) {
      for (let dx = -2; dx <= 2; dx++) setFixed(g, cx + dx, cy + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1);
    }
  }

  function alignmentPositions(ver, size) {
    if (ver === 1) return [];
    const numAlign = Math.floor(ver / 7) + 2;
    const step = Math.floor((ver * 8 + numAlign * 3 + 5) / (numAlign * 4 - 4)) * 2;
    const result = [6];
    for (let pos = size - 7; result.length < numAlign; pos -= step) result.splice(1, 0, pos);
    return result;
  }

  function isFinderCorner(i, j, last) {
    return (i === 0 && j === 0) || (i === 0 && j === last) || (i === last && j === 0);
  }

  function drawFunctionPatterns(g, ver) {
    const size = g.size;
    for (let i = 0; i < size; i++) {
      setFixed(g, 6, i, i % 2 === 0);
      setFixed(g, i, 6, i % 2 === 0);
    }
    drawFinder(g, 3, 3);
    drawFinder(g, size - 4, 3);
    drawFinder(g, 3, size - 4);
    const pos = alignmentPositions(ver, size);
    const last = pos.length - 1;
    pos.forEach((y, i) => pos.forEach((x, j) => {
      if (!isFinderCorner(i, j, last)) drawAlignment(g, x, y);
    }));
    grid.drawFormat(g, 0);
    grid.drawVersion(g, ver);
  }

  window.goneQrPatterns = Object.freeze({ drawFunctionPatterns: drawFunctionPatterns });
})();
