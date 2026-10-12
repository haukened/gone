'use strict';

// Codeword side of the QR encoder (window.goneQr): the level M tables,
// Reed-Solomon error correction, capacity, version choice, and the byte-mode
// data codewords interleaved with their error correction. Exposed as
// window.goneQrCodewords.
(function qrCodewordsModule() {
  if (window.goneQrCodewords) return;

  // Per-version tables for level M, indexed by version (index 0 unused).
  const ECC_PER_BLOCK = [-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26,
    26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28];
  const NUM_BLOCKS = [-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16,
    17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49];

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

  window.goneQrCodewords = Object.freeze({
    pickVersion: pickVersion,
    dataBytes: dataBytes,
    interleave: interleave
  });
})();
