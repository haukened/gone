'use strict';

// QR code encoder (ISO/IEC 18004) for the share link, so the result page can
// draw a code without a third-party library. It supports only what a link
// needs: byte mode, error correction level M, versions 1 to 40, and automatic
// mask selection. The structure follows Project Nayuki's reference
// implementation. Exposed as window.goneQr.
(function qrModule() {
  if (window.goneQr) return;

  // Per-version tables for level M, indexed by version (index 0 unused).
  const ECC_PER_BLOCK = [-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26,
    26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28];
  const NUM_BLOCKS = [-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16,
    17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49];
  const FORMAT_BITS_M = 0;
  const PENALTY = { run: 3, block: 3, finder: 40, balance: 10 };

  // ---- Galois field and Reed-Solomon ----

  function gfMultiply(x, y) {
    let z = 0;
    for (let i = 7; i >= 0; i--) {
      z = (z << 1) ^ ((z >>> 7) * 0x11d);
      z ^= ((y >>> i) & 1) * x;
    }
    return z;
  }

  function rsDivisor(degree) {
    const result = new Array(degree).fill(0);
    result[degree - 1] = 1;
    let root = 1;
    for (let i = 0; i < degree; i++) {
      for (let j = 0; j < degree; j++) {
        result[j] = gfMultiply(result[j], root);
        if (j + 1 < degree) result[j] ^= result[j + 1];
      }
      root = gfMultiply(root, 0x02);
    }
    return result;
  }

  function rsRemainder(data, divisor) {
    const result = divisor.map(() => 0);
    data.forEach((b) => {
      const factor = b ^ result.shift();
      result.push(0);
      divisor.forEach((coef, i) => { result[i] ^= gfMultiply(coef, factor); });
    });
    return result;
  }

  // ---- Capacity ----

  function rawDataModules(ver) {
    let result = (16 * ver + 128) * ver + 64;
    if (ver >= 2) {
      const numAlign = Math.floor(ver / 7) + 2;
      result -= (25 * numAlign - 10) * numAlign - 55;
      if (ver >= 7) result -= 36;
    }
    return result;
  }

  function dataCodewords(ver) {
    return Math.floor(rawDataModules(ver) / 8) - ECC_PER_BLOCK[ver] * NUM_BLOCKS[ver];
  }

  function countBits(ver) {
    return ver <= 9 ? 8 : 16;
  }

  // pickVersion returns the smallest version that holds n bytes, or throws.
  function pickVersion(n) {
    for (let ver = 1; ver <= 40; ver++) {
      if (4 + countBits(ver) + 8 * n <= dataCodewords(ver) * 8) return ver;
    }
    throw new RangeError('text too long for a QR code');
  }

  // ---- Data codewords ----

  function pushBits(bits, value, len) {
    for (let i = len - 1; i >= 0; i--) bits.push((value >>> i) & 1);
  }

  // dataBytes builds the byte-mode segment with terminator and padding.
  function dataBytes(bytes, ver) {
    const capacity = dataCodewords(ver) * 8;
    const bits = [];
    pushBits(bits, 0x4, 4);
    pushBits(bits, bytes.length, countBits(ver));
    bytes.forEach((b) => pushBits(bits, b, 8));
    pushBits(bits, 0, Math.min(4, capacity - bits.length));
    pushBits(bits, 0, (8 - (bits.length % 8)) % 8);
    for (let pad = 0xec; bits.length < capacity; pad ^= 0xec ^ 0x11) pushBits(bits, pad, 8);
    const out = [];
    for (let i = 0; i < bits.length; i += 8) out.push(parseInt(bits.slice(i, i + 8).join(''), 2));
    return out;
  }

  // interleave splits data into blocks, appends each block's error correction
  // and interleaves the result as the standard lays it out.
  function interleave(data, ver) {
    const numBlocks = NUM_BLOCKS[ver];
    const eccLen = ECC_PER_BLOCK[ver];
    const raw = Math.floor(rawDataModules(ver) / 8);
    const numShort = numBlocks - (raw % numBlocks);
    const shortLen = Math.floor(raw / numBlocks);
    const divisor = rsDivisor(eccLen);
    const blocks = [];
    for (let i = 0, k = 0; i < numBlocks; i++) {
      const dat = data.slice(k, k + shortLen - eccLen + (i < numShort ? 0 : 1));
      k += dat.length;
      const ecc = rsRemainder(dat, divisor);
      if (i < numShort) dat.push(0);
      blocks.push(dat.concat(ecc));
    }
    const out = [];
    for (let i = 0; i < blocks[0].length; i++) {
      blocks.forEach((block, j) => { if (i !== shortLen - eccLen || j >= numShort) out.push(block[i]); });
    }
    return out;
  }

  // ---- Matrix ----

  function newGrid(size) {
    return { size: size, dark: new Uint8Array(size * size), fixed: new Uint8Array(size * size) };
  }

  function setFixed(g, x, y, dark) {
    g.dark[y * g.size + x] = dark ? 1 : 0;
    g.fixed[y * g.size + x] = 1;
  }

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
    drawFormat(g, 0);
    drawVersion(g, ver);
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

  // ---- Penalty (mask selection) ----

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

  // chooseMask applies each mask in turn and keeps the lowest penalty.
  function chooseMask(g) {
    let best = 0;
    let bestScore = Infinity;
    for (let mask = 0; mask < 8; mask++) {
      applyMask(g, mask);
      drawFormat(g, mask);
      const score = penalty(g);
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
    const ver = pickVersion(bytes.length);
    const g = newGrid(ver * 4 + 17);
    drawFunctionPatterns(g, ver);
    drawCodewords(g, interleave(dataBytes(bytes, ver), ver));
    const forced = opts && opts.mask;
    const mask = Number.isInteger(forced) && forced >= 0 && forced < 8 ? forced : chooseMask(g);
    applyMask(g, mask);
    drawFormat(g, mask);
    return { version: ver, size: g.size, mask: mask, dark: g.dark };
  }

  window.goneQr = Object.freeze({ encode: encode });
})();
