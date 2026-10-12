'use strict';

// Mask penalty score for the QR encoder (window.goneQr): long runs, 2x2
// blocks, finder look-alikes and dark/light balance. Exposed as
// window.goneQrPenalty.
(function qrPenaltyModule() {
  if (window.goneQrPenalty) return;

  const PENALTY = { run: 3, block: 3, finder: 40, balance: 10 };

  function addRun(history, len, size) {
    history.pop();
    history.unshift(history[0] === 0 ? len + size : len);
  }

  // isFinderCore reports whether the last five runs read 1:1:3:1:1.
  function isFinderCore(h) {
    const n = h[1];
    return n > 0 && h[2] === n && h[3] === n * 3 && h[4] === n && h[5] === n;
  }

  // countFinderLike counts 1:1:3:1:1 patterns with 4 light modules on either side.
  function countFinderLike(h) {
    if (!isFinderCore(h)) return 0;
    const n = h[1];
    return (h[0] >= n * 4 && h[6] >= n ? 1 : 0) + (h[6] >= n * 4 && h[0] >= n ? 1 : 0);
  }

  function terminateRuns(color, len, history, size) {
    let run = len;
    if (color) {
      addRun(history, run, size);
      run = 0;
    }
    addRun(history, run + size, size);
    return countFinderLike(history);
  }

  // linePenalty scores one row or column: long runs and finder look-alikes.
  function linePenalty(get, size) {
    const history = [0, 0, 0, 0, 0, 0, 0];
    let score = 0;
    let color = 0;
    let len = 0;
    for (let i = 0; i < size; i++) {
      const c = get(i);
      if (c === color) {
        len++;
        score += len === 5 ? PENALTY.run : len > 5 ? 1 : 0;
        continue;
      }
      addRun(history, len, size);
      if (!color) score += countFinderLike(history) * PENALTY.finder;
      color = c;
      len = 1;
    }
    return score + terminateRuns(color, len, history, size) * PENALTY.finder;
  }

  function blockPenalty(g) {
    const s = g.size;
    const d = g.dark;
    let score = 0;
    for (let y = 0; y < s - 1; y++) {
      for (let x = 0; x < s - 1; x++) {
        const c = d[y * s + x];
        if (c === d[y * s + x + 1] && c === d[(y + 1) * s + x] && c === d[(y + 1) * s + x + 1]) score += PENALTY.block;
      }
    }
    return score;
  }

  function balancePenalty(g) {
    const total = g.size * g.size;
    const dark = g.dark.reduce((a, b) => a + b, 0);
    return (Math.ceil(Math.abs(dark * 20 - total * 10) / total) - 1) * PENALTY.balance;
  }

  function penalty(g) {
    const s = g.size;
    let score = blockPenalty(g) + balancePenalty(g);
    for (let i = 0; i < s; i++) {
      score += linePenalty((x) => g.dark[i * s + x], s);
      score += linePenalty((y) => g.dark[y * s + i], s);
    }
    return score;
  }

  window.goneQrPenalty = Object.freeze({ penalty: penalty });
})();
